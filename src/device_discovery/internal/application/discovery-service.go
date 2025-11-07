package application

import (
	"context"
	"device_discovery/internal/application/targets"
	"device_discovery/internal/domain"
	"device_discovery/internal/domain/rules"
	"device_discovery/internal/domain/services"
	"errors"
	"fmt"
	"time"
)

type DiscoveryService struct {
	repo  domain.DeviceRepository
	icmp  ICMPScanner
	ident services.DIS
}

func NewDiscoveryService(
	repo domain.DeviceRepository,
	icmp ICMPScanner,
	ident services.DIS,
) *DiscoveryService {
	return &DiscoveryService{
		repo:  repo,
		icmp:  icmp,
		ident: ident,
	}
}

// DiscoverByRule :校验规则 → 调用 targets.ExpandRanges → 把 ips 丢给 Discover
func (s *DiscoveryService) DiscoverByRule(ctx context.Context, rule rules.DiscoveryRule, hostTimeout time.Duration, concurrency int) ([]DeviceDTO, error) {

	// 展开规则的 IP 范围
	if !rule.Enabled {
		return nil, fmt.Errorf("rule [%s] is disabled", rule.Name)
	}

	// 把规则里的 []IPRange 展开成 []string IP 列表
	ips := targets.ExpandRanges(rule.Ranges)
	return s.Discover(ctx, &rule, ips, hostTimeout, concurrency)
}

// Discover 真正的发现逻辑集中在这里
func (s *DiscoveryService) Discover(ctx context.Context, rule *rules.DiscoveryRule, ips []string, hostTimeout time.Duration, concurrency int) ([]DeviceDTO, error) {
	if len(ips) == 0 {
		return nil, fmt.Errorf("no target IPs provided")
	}

	// ICMP 扫描
	icmpResult, err := s.icmp.Sweep(ctx, ips, hostTimeout, concurrency)
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
		if rule != nil {
			if tplID, ok := rule.MatchTemplate(dev); ok {
				dev.BindTemplateId(tplID)
			}
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
