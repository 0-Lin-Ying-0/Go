package repo

import (
	"context"
	"device_discovery/internal/infrastructure/persistence"
	"errors"
	"time"

	"gorm.io/gorm"
)

/*
服务于调度子系统,对 discovery_schedules 表做查询与局部更新,为“调度器加载计划、更新时间窗口、回填下次执行时间”等提供持久化能力
*/

type ScheduleRepository struct {
	db *gorm.DB
}

func NewScheduleRepository(db *gorm.DB) *ScheduleRepository {
	return &ScheduleRepository{db: db}
}

// ListEnabled 从数据库读出所有“启用中的调度计划”（enabled = true），供调度器启动或定期刷新时加载到内存，随后注册成定时任务
func (r *ScheduleRepository) ListEnabled(ctx context.Context) ([]persistence.DiscoveryScheduleModel, error) {
	var rows []persistence.DiscoveryScheduleModel
	if err := r.db.WithContext(ctx).Where("enable=?", true).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

/*
UpdateRunTimes 把某条计划的最近一次执行时间（last_run_at）写回去,并可选更新next_run_at,常在一次触发完成后调用
把“刚跑完”和“下次什么时候跑”落到 DB
*/
func (r *ScheduleRepository) UpdateRunTimes(ctx context.Context, id int64, lastRun time.Time, nextRun *time.Time) error {
	updates := map[string]any{
		"last_run_at": lastRun,
	}
	if nextRun != nil {
		updates["next_run_at"] = nextRun
	}
	return r.db.WithContext(ctx).Model(&persistence.DiscoveryScheduleModel{}).Where("id = ?", id).Updates(updates).Error
}

/*
SetNextRun 只负责修改调度计划的下一次触发时间 next_run_at。

典型场景：
调度器根据表达式（cron/interval）算出下次时间后，单独落库；
需要清空下一次时间（暂停、等待人工重排）时，把列设为 NULL。
*/
func (r *ScheduleRepository) SetNextRun(ctx context.Context, id int64, nextRun *time.Time) error {
	updates := map[string]any{
		"next_run_at": nextRun,
	}
	return r.db.WithContext(ctx).Model(&persistence.DiscoveryScheduleModel{}).Where("id = ?", id).Updates(updates).Error
}

/*
FindByID 按主键 ID 精确查询一条调度计划；

找到 → 返回这条记录的完整结构体
找不到 → 返回 nil, nil（方便上层做“是否存在”的分支）
其它错误（连接/SQL/权限…） → 返回 nil, err
*/
func (r *ScheduleRepository) FindByID(ctx context.Context, id int64) (*persistence.DiscoveryScheduleModel, error) {
	var row persistence.DiscoveryScheduleModel
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		// 约定“不存在”的哨兵错误；你用它把“未找到”和“真正出错”区分开
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}
