package main

import (
	"device_discovery/internal/application"
	"device_discovery/internal/domain/services"
	"device_discovery/internal/infrastructure/network"
	"device_discovery/internal/infrastructure/persistence"
	"device_discovery/internal/interfaces/httpAPI"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type config struct {
	dbHost, dbPort, dbUser, dbPass, dbName string
	redisAddr                              string //API 现在用不到 Redis,保留字段以后扩展
	apiAddr                                string
}

func loadConfig() config {
	return config{
		dbHost:    getEnv("DB_HOST", "mysql"),
		dbPort:    getEnv("DB_PORT", "3306"),
		dbUser:    getEnv("DB_USER", "root"),
		dbPass:    getEnv("DB_PASS", "root"),
		dbName:    getEnv("DB_NAME", "device_discovery"),
		redisAddr: getEnv("REDIS_ADDR", "redis:6379"),
		apiAddr:   getEnv("API_ADDR", ":8080"),
	}
}

// ---- DB 连接辅助函数 ----

// mustOpenDB 根据 config 拼接 DSN，并用 GORM 打开数据库。
// 若连接失败，直接 log.Fatalf 退出程序（因为没有 DB 就没法工作了）
func mustOpenDB(cfg config) *gorm.DB {
	// - charset=utf8mb4        ：支持 emoji 等多字节字符
	// - parseTime=true         ：把 MySQL 的 datetime 映射为 Go 的 time.Time
	// - loc=Local              ：使用本地时区处理时间
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.dbUser, cfg.dbPass, cfg.dbHost, cfg.dbPort, cfg.dbName)
	// gorm.Open 会返回一个 *gorm.DB 实例，后续所有数据库操作都基于它
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	return db
}

// ---- 读取环境变量的工具函数 ----

// getEnv 读取字符串环境变量，若没设置则返回默认值 def
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	// 1) 读取配置（从环境变量中读取，给出默认值）
	cfg := loadConfig()

	// 2) 打开数据库连接 + 自动迁移表结构
	//    mustOpenDB 内部会拼接 DSN，然后 gorm.Open 连接 MySQL
	db := mustOpenDB(cfg)
	if err := db.AutoMigrate(&persistence.DeviceModel{},
		&persistence.DiscoveryScheduleModel{},
		&persistence.JobRunModel{},
		&persistence.DiscoveryRuleModel{},
	); err != nil {
		log.Fatalf("AutoMigrate error: %v", err)
	}

	// 组装应用层依赖
	deviceRepo := persistence.NewDeviceRepoDB(db)
	icmp := network.NewRawICMPScanner()
	ident := services.NewSimpleIS()
	svc := application.NewDiscoveryService(deviceRepo, icmp, ident)

	// 创建 Gin 引擎
	r := gin.Default()

	// 注册 /device 相关路由
	deviceHandler := httpAPI.NewDeviceHandler(svc)
	deviceHandler.RegisterRoutes(r)

	log.Printf("HTTP API listening on %s...", cfg.apiAddr)
	if err := r.Run(cfg.apiAddr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("gin run failed: %v", err)
	}
}
