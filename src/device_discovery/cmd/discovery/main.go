package main

import (
	"context"
	"device_discovery/internal/infrastructure/persistence/repo"
	"device_discovery/internal/infrastructure/queue"
	"device_discovery/internal/infrastructure/scheduler"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"device_discovery/internal/infrastructure/persistence"
)

// 真正的调度进程
/*
启动调度器（asynq PeriodicTaskManager）→ 从 DB 读取启用的 discovery_schedules / discovery_rules → 按计划入队 asynq 任务（Redis）→ 由独立的 Worker去执行扫描
→ 本进程常驻，监听信号优雅退出。
*/

func main() {
	// 加载配置
	cfg := loadConfig()

	// 连接 MySQL
	db := mustOpenDB(cfg)
	if err := db.AutoMigrate(&persistence.DeviceModel{}, &persistence.DiscoveryScheduleModel{}, &persistence.JobRunModel{}, &persistence.DiscoveryRuleModel{}); err != nil {
		log.Fatalf("AutoMigrate error :%v", err)
	}

	// asynq 客户端，只负责入队
	queueClient := queue.New(cfg.redisAddr)
	defer queueClient.Close()

	// 调度/运行记录仓储
	scheduleRepo := repo.NewScheduleRepository(db)
	jobRunRepo := repo.NewJobRunRepository(db)

	// 构造基于 asynq PeriodicTaskManager 的调度器
	sched := scheduler.NewAsynqScheduler(cfg.redisAddr, scheduleRepo, jobRunRepo, queueClient, slog.Default())

	// 为调度器提供可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		log.Fatalf("start scheduler failed:%v", err)
	}

	// 阻塞等待退出信号
	// 创建信号通道,缓冲区=1，保证即使主协程还没来得及取，也能先存一个信号不丢
	sigCh := make(chan os.Signal, 1)
	// 捕获退出信号
	// 把指定操作系统信号转发到 sigCh
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	// 阻塞等待。直到收到上述信号之一，才往下执行
	<-sigCh
	sched.Stop()
}

type config struct {
	dbHost    string
	dbPort    string
	dbUser    string
	dbPass    string
	dbName    string
	redisAddr string
}

// 现在只读环境变量（DB/Redis），把“扫描参数类配置”彻底移出调度器入口
func loadConfig() config {
	return config{
		dbHost:    getEnv("DB_HOST", "mysql"),
		dbPort:    getEnv("DB_PORT", "3306"),
		dbUser:    getEnv("DB_USER", "root"),
		dbPass:    getEnv("DB_PASS", "root"),
		dbName:    getEnv("DB_NAME", "device_discovery"),
		redisAddr: getEnv("REDIS_ADDR", "redis:6379"),
	}
}

// 现在用 cfg 里的 env 拼 DSN
func mustOpenDB(cfg config) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local", cfg.dbUser, cfg.dbPass, cfg.dbHost, cfg.dbPort, cfg.dbName)
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
