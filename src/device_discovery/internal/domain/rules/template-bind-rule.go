package rules

import (
	"device_discovery/internal/domain"
	"strings"
)

type TemplateBindRules struct {
	ID         int64
	VendorCond string
	TypeCond   string
	OsCond     string
	TemplateID int64
}

func (r TemplateBindRules) Matches(d *domain.Device) bool {
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
		os := strings.ToLower(strings.TrimSpace(d.OsVersion))
		if os == "" || !strings.Contains(os, strings.ToLower(strings.TrimSpace(r.OsCond))) {
			return false
		}
	}
	return true
}
