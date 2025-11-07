package persistence

import "time"

type JobRunModel struct {
	ID            int64      `gorm:"primaryKey"`
	ScheduleID    int64      `gorm:"index"`
	RunID         string     `gorm:"size:128;uniqueIndex"`
	Status        string     `gorm:"size:32;index"`
	Error         *string    `gorm:"type:text"`
	EnqueuedAt    time.Time  `gorm:"type:datetime"`
	StartedAt     *time.Time `gorm:"type:datetime"`
	FinishedAt    *time.Time `gorm:"type:datetime"`
	TookMs        *int64
	DiscoveredCnt *int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (JobRunModel) TableName() string {
	return "job_runs"
}
