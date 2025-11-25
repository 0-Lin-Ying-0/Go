package repo

import (
	"context"
	"device_discovery/internal/infrastructure/persistence"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

/* 实现“任务入队日志”的幂等写入：当调度器把一次扫描任务（一次 run）成功“入队”（投到 Asynq 队列）后，就在数据库里插入一条 JobRunModel 记录，
状态为 enqueued，并且如果这条 run 已经记过了，就什么都不做（不报错、不重复插）。这保证了同一个 runID（或 (scheduleID, runID)）不会被插入多次，
便于后续用这张表追踪一次任务从 enqueued → started → finished/failed 的生命周期。*/

type JobRunRepository struct {
	db *gorm.DB
}

func NewJobRunRepository(db *gorm.DB) *JobRunRepository {
	return &JobRunRepository{
		db: db,
	}
}

//  把“一次 run 入队成功”落到数据库里的入口方法

// CreateEnqueued 的职责是幂等地新建一条 JobRunModel
func (r *JobRunRepository) CreateEnqueued(ctx context.Context, scheduleID int64, runID string, enqueuedAt time.Time) error {
	rec := &persistence.JobRunModel{
		ScheduleID: scheduleID,
		RunID:      runID,
		Status:     persistence.StatusEnqueued,
		EnqueuedAt: enqueuedAt,
	}
	// OnConflict 保证重复 run 不会插第二次，从而构成后续状态流转（running/succeeded/failed）的起点记录
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec).Error
}

// MarkStarted 把 某次运行（runID） 的数据库记录更新为“运行中”，并写入它的开始时间
func (r *JobRunRepository) MarkStarted(ctx context.Context, runID string, startedAt time.Time) error {
	return r.update(ctx, runID, map[string]any{
		"status":     persistence.StatusRunning,
		"started_at": startedAt,
	})
}

// MarkSucceeded 是把一次运行落“成功终态”,这样生命周期有了可审计的数据闭环
func (r *JobRunRepository) MarkSucceeded(ctx context.Context, runID string, finishedAt time.Time, took time.Duration, discovered int) error {
	tooks := took.Milliseconds()
	return r.update(ctx, runID, map[string]any{
		"status":         persistence.StatusSucceeded,
		"finished_at":    finishedAt,
		"took_ms":        tooks,
		"discovered_cnt": discovered,
	})
}

// MarkFailed 把一次发现任务（JobRun）的运行状态标记为“失败”，同时记录结束时间与错误信息
func (r *JobRunRepository) MarkFailed(ctx context.Context, runID string, finishedAt time.Time, errMsg string) error {
	return r.update(ctx, runID, map[string]any{
		"status":      persistence.StatusFailed,
		"finished_at": finishedAt,
		"error":       errMsg,
	})
}

// Exists 判断某个 run 是否已存在：根据 runID 在 job_run_models 表里做一次查询
func (r *JobRunRepository) Exists(ctx context.Context, runID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&persistence.JobRunModel{}).
		Where("run_id=?", runID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CountForSchedule 统计某个调度（scheduleID）下已有多少条 run 记录
func (r *JobRunRepository) CountForSchedule(ctx context.Context, scheduleID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&persistence.JobRunModel{}).
		Where("schedule_id=?", scheduleID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// HasActiveRun 检查某个调度是否还有未完成的运行(enqueued / running)
func (r *JobRunRepository) HasActiveRun(ctx context.Context, scheduleID int64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&persistence.JobRunModel{}).
		Where("schedule_id=? AND status IN ?",
			scheduleID,
			[]string{persistence.StatusEnqueued, persistence.StatusRunning},
		).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// 仓储层里通用更新
func (r *JobRunRepository) update(ctx context.Context, runID string, values map[string]any) error {
	/*
		 把一次任务 run（用 runID 唯一标识）在数据库中的指定字段批量更新掉，
		并把执行结果（是否报错、影响了几行）装进一个句柄里返回给你，用来做后续判断（Error / RowsAffected）
	*/
	res := r.db.WithContext(ctx).Model(&persistence.JobRunModel{}).
		Where("run_id = ?", runID).Updates(values)
	// 把一次任务 run（用 runID 唯一标识）在数据库中的指定字段批量更新掉，并把执行结果（是否报错、影响了几行）装进一个句柄里返回给你，用来做后续判断（Error / RowsAffected）
	if res.Error != nil {
		return res.Error
	}
	// 没有任何行被更新：要么真的没有这条 runID，要么（将来你加了状态前置条件时）不满足状态流转条件
	if res.RowsAffected == 0 {
		return errors.New("job run not found")
	}
	return nil
}
