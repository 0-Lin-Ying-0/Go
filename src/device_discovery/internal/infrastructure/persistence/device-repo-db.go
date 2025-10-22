package persistence

import (
	"device_discovery/internal/domain"
	"errors"

	"gorm.io/gorm"
)

type DeviceRepoDB struct {
	db *gorm.DB
}

func NewDeviceRepoDB(db *gorm.DB) *DeviceRepoDB {
	return &DeviceRepoDB{
		db: db,
	}
}

func (r *DeviceRepoDB) Save(d *domain.Device) error {
	if d == nil {
		return errors.New("device is nil")
	}
	// 查 IP
	exist, err := r.FindByIP(d.IpAddress)
	if err != nil {
		return err
	}
	if exist == nil {
		// 新增
		m, err := toModel(d)
		if err != nil {
			return err
		}
		if err := r.db.Create(m).Error; err != nil {
			return err
		}
		d.DeviceId = m.DeviceId
		return nil
	}
	// 更新：把领域对象的字段覆盖到已存在的记录
	exist.HostName = d.HostName
	exist.DeviceType = d.DeviceType
	exist.Vendor = d.Vendor
	exist.OsVersion = d.OsVersion
	exist.Status = d.Status
	exist.DiscoveryTime = d.DiscoveryTime
	exist.LastSeen = d.LastSeen
	exist.ProtocolSupport = d.ProtocolSupport
	exist.TemplateId = d.TemplateId

	m, err := toModel(exist)
	if err != nil {
		return err
	}
	// IP 定位更新
	return r.db.Model(&DeviceModel{}).Where("ip_address = ?", exist.IpAddress).Updates(m).Error
}

func (r *DeviceRepoDB) FindByIP(ip string) (*domain.Device, error) {
	var m DeviceModel
	err := r.db.Where("ip_address = ?", ip).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomain(&m)
}

func (r *DeviceRepoDB) FindByID(id int64) (*domain.Device, error) {
	var m DeviceModel
	err := r.db.First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toDomain(&m)
}

func (r *DeviceRepoDB) List() ([]*domain.Device, error) {
	var ms []DeviceModel
	if err := r.db.Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Device, 0, len(ms))
	for i := range ms {
		d, _ := toDomain(&ms[i])
		out = append(out, d)
	}
	return out, nil
}

func (r *DeviceRepoDB) DeleteByID(id int64) error {
	return r.db.Delete(&DeviceModel{}, id).Error
}
