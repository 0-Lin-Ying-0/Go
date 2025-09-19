package application

import (
	"device_discovery/internal/domain"
	"device_discovery/internal/domain/rules"
	"device_discovery/internal/domain/services"
	"errors"
	"fmt"
	"net"
	"time"
)

type DiscoveryService struct {
	repo  domain.DiscoveryRepository
	icmp  ICMPScannerPort
	ident services.SimpleIS
}

func NewDiscoveryService(
	repo domain.DiscoveryRepository,
	icmp ICMPScannerPort,
	ident services.SimpleIS,
) *DiscoveryService {
	return &DiscoveryService{
		repo:  repo,
		icmp:  icmp,
		ident: ident,
	}
}

// DiscoverByRule :按发现规则执行一次发现
func (s *DiscoveryService) DiscoverByRule(rule rules.DiscoveryRule) ([]DeviceDTO, error) {
	// 展开规则的 IP 范围
	if !rule.Enabled {
		return nil, fmt.Errorf("rule [%s] is disabled", rule.Name)
	}

	ips := expandRanges(rule.Ranges)
	if len(ips) == 0 {
		return nil, fmt.Errorf("rule [%s] is disabled", rule.Name)
	}

	// ICMP 扫描
	icmpResult, err := s.icmp.Sweep(ips)
	if err != nil {
		return nil, fmt.Errorf("icmp sweep error:%w", err)
	}

	now := time.Now()
	var out []DeviceDTO

	for _, res := range icmpResult {
		// 加载或创建
		dev, err := s.repo.FindByIP(res.IP)
		if err != nil {
			return nil, err
		}
		if dev == nil {
			dev = domain.NewDevice(res.IP)
			dev.DiscoveryTime = now
		}

		// 识别
		s.ident.Identify(dev, services.IdentificationInput{
			ICMPReachable: res.Reachable,
			ICMPRTT:       res.RTT,
			ICMPTTL:       res.TTL,
		})

		// 绑定
		if tplID, ok := rule.MatchTemplate(dev); ok {
			dev.BindTemplateId(tplID)
		}

		// 保存
		if err := s.repo.Save(dev); err != nil {
			continue
		}
		out = append(out, FromDevice(dev))
	}
	return out, nil
}

// ListAllDevices 列出所有设备
func (s *DiscoveryService) ListAllDevices() ([]DeviceDTO, error) {
	devs, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	out := make([]DeviceDTO, 0, len(devs))
	for _, d := range devs {
		out = append(out, FromDevice(d))
	}
	return out, nil
}

// GetDeviceByIP 按 IP 获取设备
func (s *DiscoveryService) GetDeviceByIP(ip string) (DeviceDTO, error) {
	dev, err := s.repo.FindByIP(ip)
	if err != nil {
		return DeviceDTO{}, err
	}
	if dev == nil {
		return DeviceDTO{}, errors.New("device not found")
	}

	dto := FromDevice(dev)
	return dto, nil

}

// 把"多个 IP 范围的切片"展开成"所有具体 IP 的字符串切片","把多个范围拼在一起"
func expandRanges(rgs []rules.IPRange) []string {
	var out []string
	for _, r := range rgs {
		out = append(out, expRange(r)...)
	}
	return out
}

// “把一个范围展开成所有 IPv4”
func expRange(rg rules.IPRange) []string {
	start := net.ParseIP(rg.StartIP).To4()
	end := net.ParseIP(rg.EndIP).To4()
	if start == nil || end == nil {
		return nil
	}
	cur := make(net.IP, 4)
	copy(cur, start)

	var ips []string
	for ; !ipGT(cur, end); incIPv4(cur) {
		ips = append(ips, cur.String())
	}
	return ips
}

func incIPv4(ip net.IP) {
	for i := 3; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func ipGT(a, b net.IP) bool {
	for i := 0; i < 4; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
}
