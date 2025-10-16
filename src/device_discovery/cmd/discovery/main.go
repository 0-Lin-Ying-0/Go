package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"device_discovery/internal/application"
	"device_discovery/internal/domain"
	"device_discovery/internal/domain/rules"
	"device_discovery/internal/domain/services"
	"device_discovery/internal/infrastructure/network"
	"device_discovery/internal/infrastructure/persistence"
)

func main() {
	// 加载配置
	cfg := loadConfig()

	// 连接 MySQL
	db := mustOpenDB()
	if err := db.AutoMigrate(&persistence.DeviceModel{}); err != nil {
		log.Fatalf("AutoMigrate error :%v", err)
	}

	// 组装领域依赖
	repo := persistence.NewDeviceRepoDB(db)
	icmp := network.NewRawICMPScanner()
	ident := services.NewSimpleIS()
	ds := application.NewDiscoveryService(repo, icmp, ident)

	// 构造发现
	rule := buildRule(cfg)

	// 并发扫描+识别+入库
	ctx := context.Background()
	log.Printf("start discovery: rule=%s targets=%s",
		rule.Name, cfg.targetsDisplay)
	dtos, err := ds.DiscoverByRule(ctx, rule, cfg.hostTimeout, cfg.concurrency)
	if err != nil {
		log.Fatalf("DiscoverByRule error :%v", err)
	}

	// 控制台输出结果
	printResult(dtos)
}

type config struct {
	ruleID         int64
	ruleName       string
	targetRanges   []string
	targetsDisplay string
	hostTimeout    time.Duration
	concurrency    int
}

func loadConfig() config {
	var (
		ruleID   = flag.Int64("rule-id", 1, "ID of the discovery rule(for logging only)")
		ruleName = flag.String("rule-name", "adhoc", "Name of the discovery rule")
		targets  = flag.String("targets", getEnv("DISCOVERY_TARGETS", "127.0.0.1"),
			"comma separated targets (CIDR,start-end,or single IP)")
		timeout = flag.Duration("timeout", getDurationEnv("DISCOVERY_TIMEOUT", 2*time.Second),
			"per host timeout (e.g. 2s,500ms)")
		concurrency = flag.Int("concurrency", getIntEnv("DISCOVERY_CONCURRENCY", 32),
			"max concurrent ICMP probes")
	)
	flag.Parse()

	targetList := splitAndClean(*targets)
	if len(targetList) == 0 {
		log.Fatalf("No discovery targets provided")
	}

	log.Printf("rule=%s(%d) targets=%s timeout=%s concurrency=%d",
		*ruleName, *ruleID, strings.Join(targetList, ","), *timeout, *concurrency)

	return config{
		ruleID:         *ruleID,
		ruleName:       *ruleName,
		targetRanges:   targetList,
		targetsDisplay: strings.Join(targetList, ","),
		hostTimeout:    *timeout,
		concurrency:    *concurrency,
	}
}

func buildRule(cfg config) rules.DiscoveryRule {
	rule := rules.DiscoveryRule{
		ID:        cfg.ruleID,
		Name:      cfg.ruleName,
		Enabled:   true,
		Frequency: time.Hour,
	}
	for _, rgStr := range cfg.targetRanges {
		rg, err := rules.ParseIPRange(rgStr)
		if err != nil {
			log.Fatalf("invalid target range %q: %v", rgStr, err)
		}
		rule.AddRange(rg)
	}
	rule.AddProtocol(domain.ScanProtocolICMP)
	return rule
}

//-------------------

func mustOpenDB() *gorm.DB {
	host := getEnv("DB_HOST", "mysql")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "root")
	pass := getEnv("DB_PASS", "root")
	name := getEnv("DB_NAME", "device_discovery")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		user, pass, host, port, name)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	return db
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getIntEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var parsed int
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil {
			return parsed
		}
		log.Printf("invalid integer for %s=%q,use default %d", key, v, def)
	}
	return def
}

func getDurationEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("invalid duration for %s=%q,use default %s", key, v, def)
	}
	return def
}

func splitAndClean(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func printResult(dtos []application.DeviceDTO) {
	if len(dtos) == 0 {
		log.Println("no devices discovered")
		return
	}
	data, err := json.MarshalIndent(dtos, "", " ")
	if err != nil {
		log.Printf("marshal result error: %v", err)
		return
	}
	fmt.Println(string(data))
}
