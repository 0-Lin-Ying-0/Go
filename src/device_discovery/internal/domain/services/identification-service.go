package services

import (
	"device_discovery/internal/domain"
	"strings"
	"time"
)

// IdentificationInput 是"识别设备"的输入信息
type IdentificationInput struct {
	ICMPReachable bool //能否ping通
	ICMPRTT       time.Duration
	ICMPTTL       int

	SysObjectID string //SNMP
	Banner      string //HTTP\SSH
	Hostname    string
	Extra       map[string]string
}

// DIS ：纯业务规则的领域服务接口
type DIS interface {
	Identify(d *domain.Device, in IdentificationInput)
}

// SimpleIS 一个最小可用的实现
type SimpleIS struct {
	oidVendorPrefix map[string]string
	bannerVendorSub map[string]string
	hostVendorSub   map[string]string
}

func NewSimpleIS() *SimpleIS {
	return &SimpleIS{
		oidVendorPrefix: map[string]string{
			"1.3.6.1.4.1.9":     "Cisco",
			"1.3.6.1.4.1.2011":  "Huawei",
			"1.3.6.1.4.1.2636":  "Juniper",
			"1.3.6.1.4.1.25506": "H3C",
		},
		bannerVendorSub: map[string]string{
			"cisco":   "Cisco",
			"huawei":  "Huawei",
			"juniper": "Juniper",
			"h3c":     "H3C",
			"zte":     "ZTE",
			"hp":      "HP",
		},
		hostVendorSub: map[string]string{
			"cisco":   "Cisco",
			"huawei":  "Huawei",
			"h3c":     "H3C",
			"juniper": "Juniper",
		},
	}
}

func (s *SimpleIS) Identify(d *domain.Device, in IdentificationInput) {
	if d == nil {
		return
	}

	// ICMP
	if in.ICMPReachable {
		d.TouchSeen()
		d.AddPrl(domain.ScanProtocolICMP)
	}

	// Vendor
	if d.Vendor == "" {
		if v, ok := s.matchOIDVendor(in.SysObjectID); ok {
			d.SetVendor(v)
		}
	}

	if d.Vendor == "" && in.Banner != "" {
		if v, ok := s.matchSubVendor(in.Banner, s.bannerVendorSub); ok {
			d.SetVendor(v)
		}
	}

	if d.Vendor == "" && in.Hostname != "" {
		if v, ok := s.matchSubVendor(in.Hostname, s.hostVendorSub); ok {
			d.SetVendor(v)
		}
	}

	if d.Vendor == "" {
		d.SetVendor("Unknown")
	}

	// 推断

	// 基于 ICMP TTL 的非常粗略推断（仅在 DeviceType/OsVersion 为空时尝试）
	//    - TTL≥200：很多网络设备（如 Cisco/Juniper）默认 TTL 高（255），可倾向“NetworkDevice”
	//    - 100≤TTL<200：常见 Windows 默认 TTL≈128
	//    - 40≤TTL≤80：常见 Linux/Unix 默认 TTL≈64
	if in.ICMPTTL > 0 {
		if d.DeviceType == "" {
			switch {
			case in.ICMPTTL >= 200:
				d.SetDeviceType("NetworkDevice")
			case in.ICMPTTL >= 100:
				d.SetDeviceType("Server")
			case in.ICMPTTL >= 40:
				d.SetDeviceType("Server")
			}
		}
		if d.OsVersion == "" {
			if in.ICMPTTL >= 100 && in.ICMPTTL < 200 {
				d.SetOsVersion("Windows")
			} else if in.ICMPTTL >= 40 && in.ICMPTTL < 80 {
				d.SetOsVersion("Linux/Unix")
			}
		}
	}

	// 若仍无类型信息，再用 banner/hostname 做弱提示
	if d.DeviceType == "" {
		text := strings.ToLower(in.Banner + "" + in.Hostname)
		switch {
		case strings.Contains(text, "router") || strings.Contains(text, "ios"):
			d.SetDeviceType("Router")
		case strings.Contains(text, "switch"):
			d.SetDeviceType("Switch")
		case strings.Contains(text, "server"):
			d.SetDeviceType("Server")
		}
	}

	// OSVersion：从 banner 再补一把（SNMP/SSH 更精确的可后续替换）
	if d.OsVersion == "" && in.Banner != "" {
		low := strings.ToLower(in.Banner)
		switch {
		case strings.Contains(low, "ios"):
			d.SetOsVersion("IOS")
		case strings.Contains(low, "nx-os"):
			d.SetOsVersion("NX-OS")
		case strings.Contains(low, "vrp"):
			d.SetOsVersion("VRP")
		case strings.Contains(low, "linux"):
			d.SetOsVersion("Linux")
		case strings.Contains(low, "windows"):
			d.SetOsVersion("Windows")
		}
	}
}

func (s *SimpleIS) matchOIDVendor(oid string) (string, bool) {
	if oid == "" {
		return "", false
	}
	for prefix, vendor := range s.oidVendorPrefix {
		if strings.HasPrefix(oid, prefix) {
			return vendor, true
		}
	}
	return "", false
}

func (s *SimpleIS) matchSubVendor(text string, table map[string]string) (string, bool) {
	if text == "" {
		return "", false
	}
	low := strings.ToLower(text)
	for sub, vendor := range table {
		if strings.Contains(low, sub) {
			return vendor, true
		}
	}
	return "", false
}
