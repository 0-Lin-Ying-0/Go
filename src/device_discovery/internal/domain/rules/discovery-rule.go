package rules

import (
	"device_discovery/internal/domain"
	"time"
)

type DiscoveryRule struct {
	ID               int64
	Name             string
	Enabled          bool
	Ranges           []IPRange
	ProtocolOrder    []domain.ScanProtocol
	TemplateRules    []TemplateBindRules
	Frequency        time.Duration
	OfflineThreshold time.Duration
}

func (r DiscoveryRule) ContainsIP(ip string) bool {
	for _, IPrg := range r.Ranges {
		if IPrg.Contains(ip) {
			return true
		}
	}
	return false
}

// MatchTemplate 根据设备属性，按顺序返回第一条匹配的模板ID。
func (r DiscoveryRule) MatchTemplate(d *domain.Device) (int64, bool) {
	for _, tr := range r.TemplateRules {
		if tr.Matches(d) && tr.TemplateID > 0 {
			return tr.TemplateID, true
		}
	}
	return 0, false
}

func (r *DiscoveryRule) AddRange(rg IPRange) {
	r.Ranges = append(r.Ranges, rg)
}

func (r *DiscoveryRule) AddProtocol(p domain.ScanProtocol) {
	for _, existed := range r.ProtocolOrder {
		if existed == p {
			return
		}
	}
	r.ProtocolOrder = append(r.ProtocolOrder, p)
}

func (r DiscoveryRule) HasProtocol(p domain.ScanProtocol) bool {
	for _, existed := range r.ProtocolOrder {
		if existed == p {
			return true
		}
	}
	return false
}
