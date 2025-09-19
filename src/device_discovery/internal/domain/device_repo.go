package domain

type DiscoveryRepository interface {
	Save(d *Device) error
	FindByID(id int64) (*Device, error)
	FindByIP(ip string) (*Device, error)
	List() ([]*Device, error)
	DeleteByID(id int64) error
}
