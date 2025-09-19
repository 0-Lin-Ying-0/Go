// 用 IPRange 表达扫描目标（单 IP / CIDR / 起止段）

package rules

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

type IPRange struct {
	StartIP string
	EndIP   string
}

// NewIPRangeSingle 表示“单个IP”的范围
func NewIPRangeSingle(ip string) (IPRange, error) {
	if !isIPv4(ip) {
		return IPRange{}, errors.New("invalid ip")
	}
	return IPRange{StartIP: ip, EndIP: ip}, nil
}

// NewIPRangeFromCIDR 将 CIDR（如 192.168.1.0/24）转换为 IP 范围
// 仅支持 IPv4
func NewIPRangeFromCIDR(cidr string) (IPRange, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return IPRange{}, err
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return IPRange{}, errors.New("only IPv4 is supported")
	}
	start := ipToUint32(ip4)

	//计算广播地址(结束IP)
	mask := ipNet.Mask
	bcast := (start & maskToUint32(mask)) | (^maskToUint32(mask))

	return IPRange{
		StartIP: uint32ToIP(start).String(),
		EndIP:   uint32ToIP(bcast).String(),
	}, nil
}

// Contains 判断一个 IP 是否落在此范围内（含边界）
func (r IPRange) Contains(ip string) bool {
	ip4 := parseIPv4(ip)
	if ip4 == nil {
		return false
	}
	v := ipToUint32(ip4)
	start := ipToUint32(parseIPv4(r.StartIP))
	end := ipToUint32(parseIPv4(r.EndIP))

	if start == 0 || end == 0 {
		return false
	}
	return v >= start && v <= end
}

// NewIPRangeFromStartEnd 允许显式指定起止IP（会做合法性与顺序检查）
func NewIPRangeFromStartEnd(start, end string) (IPRange, error) {
	sIP := parseIPv4(start)
	eIP := parseIPv4(end)
	if sIP == nil || eIP == nil {
		return IPRange{}, errors.New("invalid ipv4 start/end")
	}
	if ipToUint32(sIP) > ipToUint32(eIP) {
		return IPRange{}, errors.New("StartIp must be <= EndIp ")
	}
	return IPRange{StartIP: start, EndIP: end}, nil
}

// 仅判断是否是 IPv4 字符串
func isIPv4(s string) bool {
	return parseIPv4(s) != nil
}

// 将 IPv4 字符串解析为 net.IP（长度4），失败返回 nil
func parseIPv4(s string) net.IP {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return nil
	}
	return ip.To4()
}

// 将 IPv4（长度4）转换为 uint32
func ipToUint32(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// 将掩码转为 uint32
func maskToUint32(m net.IPMask) uint32 {
	return uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3])
}

// 将 uint32 转回 IPv4
func uint32ToIP(v uint32) net.IP {
	return net.IP{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func ParseIPRange(s string) (IPRange, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		if len(parts) != 2 {
			return IPRange{}, errors.New("invalid ip range")
		}
		return NewIPRangeFromStartEnd(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
	if strings.Contains(s, "/") {
		return NewIPRangeFromCIDR(strings.TrimSpace(s))
	}
	return NewIPRangeSingle(strings.TrimSpace(s))
}

// Size 估算范围内的IP数量
func (r IPRange) Size() int {
	start := ipToUint32(parseIPv4(r.StartIP))
	end := ipToUint32(parseIPv4(r.EndIP))
	if start == 0 || end == 0 || start > end {
		return 0
	}
	return int(end - start + 1)
}

func (r IPRange) Log() string {
	return r.StartIP + "-" + r.EndIP + "(" + (strconv.Itoa(r.Size())) + ")"
}
