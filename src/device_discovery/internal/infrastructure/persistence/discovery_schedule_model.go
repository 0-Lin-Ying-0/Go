package persistence

import "time"

type DiscoveryScheduleModel struct {
	ID              int64  `gorm:"primary_Key"`
	RuleID          int64  `gorm:"index"`
	Type            string `gorm:"size:32;not null"`
	Expression      string `gorm:"type:text;not null"`
	Timezone        string `gorm:"size:64;not null"`
	Enable          bool   `gorm:"index"`
	OverlapPolicy   string `gorm:"size:16;not null"`
	Incremental     bool
	FullEveryNRuns  *int
	HostTimeoutSecs int        `gorm:"default:2"`
	Concurrency     int        `gorm:"default:32"`
	LastRunAt       *time.Time `gorm:"type:datetime"`
	NextRunAt       *time.Time `gorm:"type:datetime"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName 覆盖默认表名
func (DiscoveryScheduleModel) TableName() string {
	return "discovery_schedules"
}
