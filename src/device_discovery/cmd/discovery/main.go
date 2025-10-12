package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
	// 1) 连接 MySQL
	db := mustOpenDB()
	// 自动建表
	if err := db.AutoMigrate(&persistence.DeviceModel{}); err != nil {
		log.Fatalf("AutoMigrate error: %v", err)
	}

	// 2) 组装依赖
	repo := persistence.NewDeviceRepoDB(db)
	icmp := network.NewRawICMPScanner()
	ident := services.NewSimpleIS()
	ds := application.NewDiscoveryService(repo, icmp, ident)

	// 3) HTTP 路由
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	// POST /discover?cidr=192.168.1.0/30
	http.HandleFunc("/discover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cidr := r.URL.Query().Get("cidr")
		if cidr == "" {
			http.Error(w, "cidr required", http.StatusBadRequest)
			return
		}
		rg, err := rules.NewIPRangeFromCIDR(cidr)
		if err != nil {
			http.Error(w, "invalid cidr", http.StatusBadRequest)
			return
		}
		rule := rules.DiscoveryRule{
			ID:        1,
			Name:      fmt.Sprintf("adhoc-%s", cidr),
			Enabled:   true,
			Frequency: time.Hour, // 占位
		}
		rule.AddRange(rg)
		rule.AddProtocol(domain.ScanProtocolICMP)
		// 可选：也可以配置 TemplateRules

		// 调用应用服务
		ctx := r.Context()
		hostTimeout := 2 * time.Second
		concurrency := 100

		dtos, err := ds.DiscoverByRule(ctx, rule, hostTimeout, concurrency)
		if err != nil {
			http.Error(w, "discover error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, dtos)
	})

	addr := ":8080"
	log.Printf("service listening at %s ...", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

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

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
