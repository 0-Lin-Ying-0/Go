package persistence

import (
	"device_discovery/internal/domain"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type DeviceModel struct {
	DeviceID        int64      `gorm:"primaryKey;autoIncrement"`
	IPAddress       string     `gorm:"size:64;uniqueIndex;not null"`
	HostName        string     `gorm:"size:255"`
	DeviceType      string     `gorm:"size:128"`
	Vendor          string     `gorm:"size:128"`
	OSVersion       string     `gorm:"size:128"`
	Status          string     `gorm:"size:32;index"`
	DiscoveryTime   time.Time  `gorm:"type:datetime;index"`
	LastSeen        *time.Time `gorm:"type:datetime"`
	ProtocolSupport string     `gorm:"type:text"`
	TemplateID      int64      `gorm:"index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

// Domain - Model
func toModel(d *domain.Device) (*DeviceModel, error) {
	bs, err := json.Marshal(d.ProtocolSupport)
	if err != nil {
		return nil, err
	}
	m := &DeviceModel{
		DeviceID:        d.DeviceID,
		IPAddress:       d.IPAddress,
		HostName:        d.HostName,
		DeviceType:      d.DeviceType,
		Vendor:          d.Vendor,
		OSVersion:       d.OSVersion,
		Status:          string(d.Status),
		DiscoveryTime:   d.DiscoveryTime,
		ProtocolSupport: string(bs),
		TemplateID:      d.TemplateId,
	}
	// LastSeen 允许为空
	if !d.LastSeen.IsZero() {
		t := d.LastSeen
		m.LastSeen = &t
	}
	return m, nil
}

// Model - Domain
func toDomain(m *DeviceModel) (*domain.Device, error) {
	var protos []domain.ScanProtocol
	if m.ProtocolSupport != "" {
		_ = json.Unmarshal([]byte(m.ProtocolSupport), &protos)
	}
	var lastSeen time.Time
	if m.LastSeen != nil {
		lastSeen = *m.LastSeen
	}
	return &domain.Device{
		DeviceID:        m.DeviceID,
		IPAddress:       m.IPAddress,
		HostName:        m.HostName,
		DeviceType:      m.DeviceType,
		Vendor:          m.Vendor,
		OSVersion:       m.OSVersion,
		Status:          domain.DeviceStatus(m.Status),
		DiscoveryTime:   m.DiscoveryTime,
		LastSeen:        lastSeen,
		ProtocolSupport: protos,
		TemplateId:      m.TemplateID,
	}, nil
}
