package application

import (
	"context"
	"time"
)

type ICMPResult struct {
	IP        string
	Reachable bool
	RTT       time.Duration
	TTL       int
}

//type ICMPScannerPort interface {
//	Ping(ip string) (ICMPResult, error)
//	Sweep(ips []string) ([]ICMPResult, error)
//}

type ICMPScanner interface {
	Ping(ctx context.Context, ip string, timeout time.Duration) (ICMPResult, error)
	Sweep(ctx context.Context, ip []string, timeout time.Duration, maxConcurrency int) ([]ICMPResult, error)
}
