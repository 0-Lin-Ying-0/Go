package domain

import "time"

type DeviceStatus string

const (
	StatusUnknown DeviceStatus = "unknown"
	StatusOnline  DeviceStatus = "online"
	StatusOffline DeviceStatus = "offline"
)

type ScanProtocol string

const (
	ScanProtocolICMP   ScanProtocol = "ICMP"
	ScanProtocolSNMP   ScanProtocol = "SNMP"
	ScanProtocolHTTP   ScanProtocol = "HTTP"
	ScanProtocolSSH    ScanProtocol = "SSH"
	ScanProtocolTelnet ScanProtocol = "Telnet"
)

type Device struct {
	DeviceId        int64
	IpAddress       string
	HostName        string
	DeviceType      string
	Vendor          string
	OsVersion       string
	Status          DeviceStatus
	DiscoveryTime   time.Time
	LastSeen        time.Time
	ProtocolSupport []ScanProtocol
	TemplateId      int64
}

func NewDevice(ipAddress string) *Device {
	return &Device{
		IpAddress:     ipAddress,
		Status:        StatusUnknown,
		DiscoveryTime: time.Now(),
		// 其他字段留空，待后续识别/探测补全
	}
}

// NewD 用于判断“是否为新设备”（尚未持久化）。
// 领域内常用来决定“新增还是更新”，或是否触发“新设备工作流”。
func (d *Device) NewD() bool {
	return d.DeviceId == 0
}

// SupportsPrl 判断设备是否“已知支持”某协议
// 仅基于本实体字段做判断（纯领域行为）
func (d *Device) SupportsPrl(p ScanProtocol) bool {
	for _, ps := range d.ProtocolSupport {
		if ps == p {
			return true
		}
	}
	return false
}

// AddPrl 为设备记录一个“已知支持的协议”
// 会自动去重：若已记录则不重复添加
func (d *Device) AddPrl(p ScanProtocol) {
	if !d.SupportsPrl(p) {
		d.ProtocolSupport = append(d.ProtocolSupport, p)
	}
}

// RemovePrl 从“已知支持的协议”中移除一个协议
func (d *Device) RemovePrl(p ScanProtocol) {
	if len(d.ProtocolSupport) == 0 {
		return
	}
	out := make([]ScanProtocol, 0, len(d.ProtocolSupport))
	for _, ps := range d.ProtocolSupport {
		if ps != p {
			out = append(out, ps)
		}
	}
	d.ProtocolSupport = out
}

// TouchSeen 标记“刚刚确认在线”
func (d *Device) TouchSeen() {
	d.Status = StatusOnline
	d.LastSeen = time.Now()
}

// MarkOffline 将设备标记为离线（不改 LastSeen，保留最后一次在线时间）
func (d *Device) MarkOffline() {
	d.Status = StatusOffline
}

func (d *Device) SetDeviceType(t string) {
	d.DeviceType = t
}

func (d *Device) SetVendor(v string) {
	d.Vendor = v
}

func (d *Device) SetOsVersion(ov string) {
	d.OsVersion = ov
}

func (d *Device) BindTemplateId(id int64) {
	d.TemplateId = id
}
