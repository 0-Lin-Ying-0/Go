package targets

import (
	"device_discovery/internal/domain/rules"
	"net"
)

// 纯函数工具：把范围“展开成 IP 列表”

// ExpandRanges 是把“规则里的多个范围”逐个展开、汇总成一串 IP 字符串的入口工具
func ExpandRanges(ranges []rules.IPRange) []string {
	var out []string
	for _, rg := range ranges {
		out = append(out, expandRange(rg)...)
	}
	return out
}

// expandRange 就是把一个“起止段”范围（StartIP → EndIP）展开成逐个 IPv4 地址
func expandRange(rg rules.IPRange) []string {
	// net.ParseIP(s)：把字符串解析成 net.IP（字节切片）
	// .To4()：转成 4 字节的 IPv4 表示（不是 IPv4 返回 nil）
	start := net.ParseIP(rg.StartIP).To4()
	end := net.ParseIP(rg.EndIP).To4()
	if start == nil || end == nil {
		return nil
	}

	// 拷贝一份“可变的当前指针 cur”，避免直接在 start 上原地自增
	cur := make(net.IP, len(start))
	copy(cur, start)

	// 从 start 开始，每次 +1 一个地址，直到 cur > end 才停止，因此是包含 end 的闭区间
	var ips []string
	for ; !ipGreaterThan(cur, end); incrementIPv4(cur) {
		ips = append(ips, cur.String())
	}
	return ips
}

func incrementIPv4(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- { // 从最后一位向前
		ip[i]++         // 该位 +1（byte 溢出会回到 0）
		if ip[i] != 0 { // 没有溢出则完成
			break // 退出循环
		}
		// 否则继续下一位（进位）
	}
}

// 比较两个 IPv4 的大小关系
func ipGreaterThan(a, b net.IP) bool {
	for i := 0; i < len(a); i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
}
