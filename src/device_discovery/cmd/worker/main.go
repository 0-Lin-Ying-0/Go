package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"device_discovery/internal/application"
	"device_discovery/internal/application/targets"
	"device_discovery/internal/domain/services"
	"device_discovery/internal/infrastructure/network"
	"device_discovery/internal/infrastructure/persistence"
	"device_discovery/internal/infrastructure/persistence/repo"
	"device_discovery/internal/infrastructure/queue"

	"github.com/hibiken/asynq"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

/*
它是“发现任务”的执行器：从 Redis 队列里拿到一条“discover:run”任务 → 查规则 → 选目标（支持增量）
→ 依配置并发 ICMP 扫描与识别 → 把执行结果写数据库（成功/失败/统计），并把整次任务的生命周期状态落库可追踪。
*/

func main() {
	cfg := loadConfig()

	db := mustOpenDB(cfg)
	if err := db.AutoMigrate(&persistence.DeviceModel{}, &persistence.DiscoveryScheduleModel{}, &persistence.JobRunModel{}, &persistence.DiscoveryRuleModel{}); err != nil {
		log.Fatalf("AutoMigrate error: %v", err)
	}

	ruleRepo := repo.NewRuleRepository(db)
	jobRunRepo := repo.NewJobRunRepository(db)
	provider := targets.NewDBTargetProvider(db, ruleRepo, cfg.incrementalTTL)

	repoDevice := persistence.NewDeviceRepoDB(db)
	icmp := network.NewRawICMPScanner()
	ident := services.NewSimpleIS()
	discoverySvc := application.NewDiscoveryService(repoDevice, icmp, ident)

	redisOpt := asynq.RedisClientOpt{Addr: cfg.redisAddr}
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.workerConcurrency,
		Queues:      map[string]int{"discover": 10},
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc("discover:run", func(ctx context.Context, task *asynq.Task) error {
		var payload queue.DiscoverPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		exists, err := jobRunRepo.Exists(ctx, payload.RunID)
		if err != nil {
			return err
		}
		if !exists {
			if err := jobRunRepo.CreateEnqueued(ctx, payload.ScheduleID, payload.RunID, time.Now()); err != nil {
				return err
			}
		}

		if err := jobRunRepo.MarkStarted(ctx, payload.RunID, time.Now()); err != nil {
			return err
		}

		rule, err := ruleRepo.FindByID(ctx, payload.RuleID)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}
		if rule == nil {
			err := fmt.Errorf("rule %d not found", payload.RuleID)
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}

		targetsList, err := provider.TargetsForRule(ctx, payload.RuleID, payload.Incremental)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}

		timeoutSecs, concurrency, err := ruleRepo.LoadExecutionConfig(ctx, payload.RuleID)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}

		start := time.Now()
		dtos, err := discoverySvc.Discover(ctx, rule, targetsList, time.Duration(timeoutSecs)*time.Second, concurrency)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}
		duration := time.Since(start)
		count := len(dtos)
		if err := jobRunRepo.MarkSucceeded(ctx, payload.RunID, time.Now(), duration, count); err != nil {
			return err
		}
		return nil
	})

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("worker stopped: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	srv.Shutdown()
}

type config struct {
	dbHost            string
	dbPort            string
	dbUser            string
	dbPass            string
	dbName            string
	redisAddr         string
	workerConcurrency int
	incrementalTTL    time.Duration
}

func loadConfig() config {
	return config{
		dbHost:            getEnv("DB_HOST", "mysql"),
		dbPort:            getEnv("DB_PORT", "3306"),
		dbUser:            getEnv("DB_USER", "root"),
		dbPass:            getEnv("DB_PASS", "root"),
		dbName:            getEnv("DB_NAME", "device_discovery"),
		redisAddr:         getEnv("REDIS_ADDR", "redis:6379"),
		workerConcurrency: getIntEnv("WORKER_CONCURRENCY", 10),
		incrementalTTL:    getDurationEnv("DISCOVERY_INCREMENTAL_TTL", 24*time.Hour),
	}
}

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
