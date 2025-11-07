package repo

import (
	"context"
	"device_discovery/internal/domain/rules"
	"device_discovery/internal/infrastructure/persistence"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

/*
规则仓储的数据库实现:把“发现规则（DiscoveryRule）”这个领域对象，和数据库里的规则表互相转换、读写、校对
*/

type RuleRepository struct {
	db *gorm.DB
}

func NewRuleRepository(db *gorm.DB) *RuleRepository {
	return &RuleRepository{
		db: db,
	}
}

// FindByID 是按主键读取一条规则。从数据库把 discovery_rules 表中的记录查出来，还原成领域对象 rules.DiscoveryRule 用于后续调度执行
func (r *RuleRepository) FindByID(ctx context.Context, id int64) (*rules.DiscoveryRule, error) {
	var model persistence.DiscoveryRuleModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		// 确实没有这条记录 → 返回 (nil, nil)，交由上层用“空对象”来判断不存在
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainRule(model)
}

/*
	把数据库模型 persistence.DiscoveryRuleModel 转成领域对象 rules.DiscoveryRule

把表里存的 RangesJSON（字符串）反序列化成 []rules.IPRange，这样上层（调度/扫描服务）就能直接用 Go 的切片做逻辑，而不关心 JSON/GORM 这些底层细节
*/
func toDomainRule(model persistence.DiscoveryRuleModel) (*rules.DiscoveryRule, error) {
	var ranges []rules.IPRange
	if err := json.Unmarshal([]byte(model.RangesJson), &ranges); err != nil {
		return nil, fmt.Errorf("unmarshal ranges:%w", err)
	}
	return &rules.DiscoveryRule{
		ID:      model.ID,
		Name:    model.Name,
		Enabled: model.Enabled,
		Ranges:  ranges,
	}, nil
}

/*
Save 把领域对象 rules.DiscoveryRule（含 Name/Enabled/Ranges）和执行参数
（hostTimeoutSecs/concurrency）持久化到数据库的 discovery_rules 表里
它既能新建（ID=0 → Create），也能更新（ID≠0 → Save），从而支撑：
后台/脚本创建或修改规则；
调度热路径读取到最新规则与执行参数
*/
func (r *RuleRepository) Save(ctx context.Context, rule *rules.DiscoveryRule, hostTimeoutSecs int, concurrency int) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}
	rangesJSON, err := json.Marshal(rule.Ranges)
	if err != nil {
		return err
	}
	model := &persistence.DiscoveryRuleModel{
		ID:              rule.ID,
		Name:            rule.Name,
		Enabled:         rule.Enabled,
		RangesJson:      string(rangesJSON),
		HostTimeoutSecs: hostTimeoutSecs,
		Concurrency:     concurrency,
	}

	if model.ID == 0 {
		return r.db.WithContext(ctx).Create(model).Error
	}
	return r.db.WithContext(ctx).Save(model).Error
}

/*
	在调度触发的热路径里，只把“怎么扫”的两个执行参数取出来：HostTimeoutSecs、Concurrency。

这样避免把整条规则（含大 JSON 的 RangesJSON）都查出来，更轻更快
*/
func (r *RuleRepository) LoadExecutionConfig(ctx context.Context, id int64) (hostTimeoutSecs int, concurrency int, err error) {
	var model persistence.DiscoveryRuleModel
	if err = r.db.WithContext(ctx).
		// 只选必要列，减少 I/O 与解码成本
		Select("id", "host_timeout_secs", "concurrency").
		First(&model, id).Error; err != nil {
		return 0, 0, nil
	}
	return model.HostTimeoutSecs, model.Concurrency, nil
}
