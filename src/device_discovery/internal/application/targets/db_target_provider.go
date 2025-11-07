package targets

import (
	"context"
	"device_discovery/internal/infrastructure/persistence"
	"device_discovery/internal/infrastructure/persistence/repo"
	"fmt"
	"time"

	"gorm.io/gorm"
)

//把规则ID→查库拿到规则→把规则里的范围展开成 IP 列表→（可选）做“增量筛选”→返回最终目标清单

/*
DBTargetProvider 是你 targets.Provider 接口的数据库实现：
给它一个 ruleID，它会 ---
从 规则仓储把规则取出来 → 用 range 展开器把范围变成一串 IP →（若增量模式）结合 设备表的 LastSeen 做“只扫新/久未见”的筛选 → 把最终 目标清单 交给后续扫描器。
这样上层完全不用关心数据来源细节，后续要换来源（文件、API）也只需再实现一个 Provider。
*/
type DBTargetProvider struct {
	db       *gorm.DB             // 查设备表( device_models),拿 LastSeen
	rules    *repo.RuleRepository // 查规则表,用你的 RuleRepository（FindByID）读出规则定义（含 Ranges、Enabled）
	staleFor time.Duration        // 增量的过期阈值,“多久未见”就视为陈旧，需要重扫
}

func NewDBTargetProvider(db *gorm.DB, rules *repo.RuleRepository, stale time.Duration) *DBTargetProvider {
	return &DBTargetProvider{
		db:       db,
		rules:    rules,
		staleFor: stale,
	}
}

func (p *DBTargetProvider) TargetsForRule(ctx context.Context, ruleID int64, incremental bool) ([]string, error) {
	// 异常 → 不存在 → 禁用，直接结束，避免后续做无用功
	rule, err := p.rules.FindByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, fmt.Errorf("rule %d not found", ruleID)
	}
	if !rule.Enabled {
		return nil, fmt.Errorf("rule %d is disabled", ruleID)
	}
	// 展开规则里的范围得到全部 IP；全量时直接返回
	ips := ExpandRanges(rule.Ranges)
	if !incremental {
		return ips, nil
	}

	// 计算“过期阈值”； 把这些 IP 在设备表里的记录一次性拉出（带上 ctx 以便取消/超时）
	cutoff := time.Now().Add(-p.staleFor)
	var rows []persistence.DeviceModel
	// 一次把这些 IP 的 LastSeen 扫出来做增量筛选
	if err := p.db.WithContext(ctx).Where("ip_address IN ?", ips).Find(&rows).Error; err != nil {
		return nil, err
	}

	// 建立 ip → 最近见到时间 的索引（nil 代表从未见过）
	lastSeen := make(map[string]*time.Time, len(rows))
	for i := range rows {
		lastSeen[rows[i].IPAddress] = rows[i].LastSeen
	}

	// 增量规则：未见过 或 早于 cutoff → 加入目标；其余近期见过 → 跳过。
	targets := make([]string, 0, len(ips))
	for _, ip := range ips {
		seen, ok := lastSeen[ip]
		if !ok || seen == nil {
			targets = append(targets, ip) // 新面孔
			continue
		}
		// 等价于：最后一次见到 < 过期阈值 → 该重扫
		if seen.Before(cutoff) {
			targets = append(targets, ip) // 老面孔，很久不见
		}
	}
	return targets, nil
}
