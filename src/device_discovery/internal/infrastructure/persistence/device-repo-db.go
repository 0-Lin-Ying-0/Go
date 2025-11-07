package persistence

import (
	"device_discovery/internal/domain"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	exist, err := r.FindByIP(d.IPAddress)
	if err != nil {
		return err
	}
	// 先假设要写入的数据来自当前扫描结果
	target := d
	// 存在旧记录则在内存中先合并字段，随后统一走 toModel(target)
	if exist != nil {
		// 更新：把领域对象的字段覆盖到已存在的记录
		exist.HostName = d.HostName
		exist.DeviceType = d.DeviceType
		exist.Vendor = d.Vendor
		exist.OSVersion = d.OSVersion
		exist.Status = d.Status
		exist.DiscoveryTime = d.DiscoveryTime
		exist.LastSeen = d.LastSeen
		exist.ProtocolSupport = d.ProtocolSupport
		exist.TemplateId = d.TemplateId
		target = exist
	}
	m, err := toModel(target)

	if err != nil {
		return err
	}
	// IP 定位更新
	if err := r.db.Clauses(clause.OnConflict{
		// 声明冲突目标 Columns: ip_address
		Columns: []clause.Column{{Name: "ip_address"}},
		// 并在冲突时更新指定列
		DoUpdates: clause.AssignmentColumns([]string{ //AssignmentColumns 明确列清单，确保零值也更新
			"host_name",
			"device_type",
			"vendor",
			"os_version",
			"status",
			"discovery_time",
			"last_seen",
			"protocol_support",
			"template_id",
			"updated_at",
		}),
	}).Create(&m).Error; err != nil {
		return err
	}
	if d.DeviceID == 0 {
		// 把数据库主键回写到领域对象，方便后续链路使用
		d.DeviceID = m.DeviceID
	}
	return nil
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
