package network

import (
	"bytes"
	"device_discovery/internal/application"
	"errors"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

type ICMPScanner struct {
	Count   int
	TimeOut time.Duration
}

func NewICMPScanner() *ICMPScanner {
	return &ICMPScanner{
		Count:   1,
		TimeOut: 2 * time.Second,
	}
}

var (
	reTTL  = regexp.MustCompile(`ttl[=\s:]?(\d+)`)
	reTime = regexp.MustCompile(`time[=\s:]?([\d\.]+)\s*ms`)
)

// Ping 单 Ip
func (i *ICMPScanner) Ping(c context.Context, ip string) (application.ICMPResult, error) {
	ctx, cancel := context.WithTimeout(c, i.TimeOut)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.CommandContext(ctx, "ping", "-c", toStr(i.Count), "-W", toStr(int(i.TimeOut.Seconds())), ip)
	case "darwin":
		cmd = exec.CommandContext(ctx, "ping", "-c", toStr(i.Count), "-W", toStr(int(i.TimeOut.Milliseconds())), ip)
	case "windows":
		cmd = exec.CommandContext(ctx, "ping", "-n", toStr(i.Count), "-w", toStr(int(i.TimeOut.Milliseconds())), ip)
	default:
		return application.ICMPResult{}, errors.New("unsupported system")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String() + "\n" + stderr.String()

	res := application.ICMPResult{IP: ip, Reachable: err == nil}
}

func toStr(i int) string {
	return strconv.Itoa(i)
}
