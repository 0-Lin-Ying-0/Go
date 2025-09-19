package application

import "time"

type ICMPResult struct {
	IP        string
	Reachable bool
	RTT       time.Duration
	TTL       int
}

type ICMPScannerPort interface {
	ping(ip string) (ICMPResult, error)
	Sweep(ips []string) ([]ICMPResult, error)
}
