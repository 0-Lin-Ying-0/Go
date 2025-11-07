package rules

import (
	"device_discovery/internal/domain"
	"strings"
)

type TemplateBindRule struct {
	ID         int64
	VendorCond string
	TypeCond   string
	OsCond     string
	TemplateID int64
}

// 省略具体实现：满足所有启用的条件就命中
func (r TemplateBindRule) Matches(d *domain.Device) bool {
	if d == nil {
		return false
	}
	if r.VendorCond != "" &&
		!strings.EqualFold(strings.TrimSpace(d.Vendor), strings.TrimSpace(r.VendorCond)) {
		return false
	}
	if r.TypeCond != "" &&
		!strings.EqualFold(strings.TrimSpace(d.DeviceType), strings.TrimSpace(r.TypeCond)) {
		return false
	}
	if r.OsCond != "" {
		os := strings.ToLower(strings.TrimSpace(d.OSVersion))
		if os == "" || !strings.Contains(os, strings.ToLower(strings.TrimSpace(r.OsCond))) {
			return false
		}
	}
	return true
}
