package application

import (
	"device_discovery/internal/domain"
	"time"
)

type DeviceDTO struct {
	DeviceId        int64     `json:"device_id"`
	IpAddress       string    `json:"ip_address"`
	HostName        string    `json:"host_name"`
	DeviceType      string    `json:"device_type"`
	Vendor          string    `json:"vendor"`
	OsVersion       string    `json:"os_version"`
	Status          string    `json:"status"`
	DiscoveryTime   time.Time `json:"discovery_time"`
	LastSeen        time.Time `json:"last_seen"`
	ProtocolSupport []string  `json:"protocol_support"`
	TemplateId      int64     `json:"template_id"`
}

func FromDevice(d *domain.Device) DeviceDTO {
	protos := make([]string, 0, len(d.ProtocolSupport))
	for _, p := range d.ProtocolSupport {
		protos = append(protos, string(p))
	}
	return DeviceDTO{
		DeviceId:        d.DeviceId,
		IpAddress:       d.IpAddress,
		HostName:        d.HostName,
		DeviceType:      d.DeviceType,
		Vendor:          d.Vendor,
		OsVersion:       d.OsVersion,
		Status:          string(d.Status),
		DiscoveryTime:   d.DiscoveryTime,
		LastSeen:        d.LastSeen,
		ProtocolSupport: protos,
		TemplateId:      d.TemplateId,
	}
}
