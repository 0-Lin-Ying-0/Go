package persistence

import "time"

type DiscoveryRuleModel struct {
	ID              int64  `gorm:"primaryKey"`
	Name            string `gorm:"size:128;not null"`
	Enabled         bool   `gorm:"index"`
	RangesJson      string `gorm:"type:text;not null"`
	HostTimeoutSecs int    `gorm:"default:2"`
	Concurrency     int    `gorm:"default:32"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (DiscoveryRuleModel) TableName() string {
	return "discovery_rules"
}
