package network

import (
	"context"
	"device_discovery/internal/application"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type RawICMPScanner struct{}

func NewRawICMPScanner() *RawICMPScanner {
	return &RawICMPScanner{}
}

// Ping 对单个 IP 执行 ICMP Echo 探测
func (s *RawICMPScanner) Ping(ctx context.Context, ip string, timeout time.Duration) (application.ICMPResult, error) {
	result := application.ICMPResult{IP: ip}
	// 1)监听 ICMP 协议，打开 RAW ICMP
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return result, fmt.Errorf("无法监听原始套接字: %w", err)
	}
	defer conn.Close()

	// 2)获取 PacketConn 以接收控制报文,用 icmp.PacketConn 的方法获得 ipv4.PacketConn
	pconn := conn.IPv4PacketConn()
	if pconn == nil {
		return result, fmt.Errorf("无法获取 IPv4 PacketConn")
	}

	// 设置控制消息以获得TTL
	_ = pconn.SetControlMessage(ipv4.FlagTTL, true)

	// 3)构造 ICMP Echo 请求消息
	sentID := os.Getpid() & 0xffff
	sentSeq := 1

	echo := &icmp.Echo{
		ID:   sentID,
		Seq:  sentSeq,
		Data: []byte("PING"),
	}
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: echo,
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return result, fmt.Errorf("ICMP 消息封包失败: %w", err)
	}

	// 4)发送 ICMP 请求
	dst := &net.IPAddr{IP: net.ParseIP(ip)}
	start := time.Now()
	// 使用 pconn 发送，可以附带控制信息
	if _, err = pconn.WriteTo(wb, nil, dst); err != nil {
		return result, nil
	}

	// 5)等待回复或超时
	buf := make([]byte, 1500)
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return result, fmt.Errorf("读取失败：%w", err)
	}

	// 读取时获取对端地址和控制信息
	n, cm, peer, err := pconn.ReadFrom(buf)
	if err != nil {
		// 读取超时或者上下文取消
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// 超时，主机可能不可达
			result.Reachable = false
			return result, nil
		}
		// 上下文取消导致结束
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, nil
	}

	// 6)验证回复来源IP是否正常
	if peerAddr, ok := peer.(*net.IPAddr); ok && !peerAddr.IP.Equal(net.ParseIP(ip)) {
		// 来源IP不匹配，忽略这个包
		result.Reachable = false
		return result, nil
	}

	// 7)解析响应,使用协议号1表示ICMPv4
	resp, err := icmp.ParseMessage(1, buf[:n])
	if err != nil {
		return result, fmt.Errorf("解析 ICMP 响应失败：%w", err)
	}

	// 先安全取 TTL
	ttl := 0
	if cm != nil {
		ttl = cm.TTL
	}

	// 8)记录结果
	if resp.Type == ipv4.ICMPTypeEchoReply {
		if e, ok := resp.Body.(*icmp.Echo); ok {
			if e.ID == sentID && e.Seq == sentSeq {
				result.Reachable = true
				result.RTT = time.Since(start)
				result.TTL = ttl
			}
		}
	}
	return result, nil
}

// Sweep 并发扫描多个 IP 列表
func (s *RawICMPScanner) Sweep(ctx context.Context, ips []string, timeout time.Duration, concurrency int) ([]application.ICMPResult, error) {
	var wg sync.WaitGroup
	resultsCh := make(chan application.ICMPResult, len(ips))
	// 并发信号量
	sem := make(chan struct{}, concurrency)

	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()

			// 获取并发信号或退出
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			// 归还名额
			defer func() { <-sem }()

			// Ping
			res, err := s.Ping(ctx, ip, timeout)
			if err != nil {
				return
			}
			resultsCh <- res
		}(ip)
	}

	// 等待所有 goroutine 完成后关闭通道
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// 结果
	var results []application.ICMPResult
	for r := range resultsCh {
		results = append(results, r)
	}
	return results, nil
}
