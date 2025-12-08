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

	// 3) 依赖注入
	ruleRepo := repo.NewRuleRepository(db)     // 规则仓库：负责读写 discovery_rules 表
	jobRunRepo := repo.NewJobRunRepository(db) // JobRun 仓库：记录每次运行状态(enqueued / started / failed / succeeded)
	// TargetProvider：从数据库中根据规则选目标（支持增量/全量逻辑）
	// incrementalTTL 是“增量有效期”：比如 24h 内扫过的设备可以跳过
	provider := targets.NewDBTargetProvider(db, ruleRepo, cfg.incrementalTTL)

	// Device 仓库：负责设备表的增删改查
	repoDevice := persistence.NewDeviceRepoDB(db)
	// ICMP 扫描器：封装底层 raw socket ping
	icmp := network.NewRawICMPScanner()
	// 设备识别服务：根据 ping 的返回信息去做简单的识别（厂商/OS 等）
	ident := services.NewSimpleIS()
	// 领域服务：把设备仓库 + 扫描器 + 识别服务串在一起
	// DiscoverService 会负责“给定规则 + 目标列表 → 执行并发扫描 → 更新设备表 → 返回 DTO 列表”
	discoverySvc := application.NewDiscoveryService(repoDevice, icmp, ident)

	// 4) 初始化 asynq Server：真正的 Worker 进程
	// redisOpt 告诉 asynq 要连接哪个 Redis（地址/密码/DB 等）
	redisOpt := asynq.RedisClientOpt{Addr: cfg.redisAddr}
	// NewServer 会启动一个基于 Redis 的任务 Worker：
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.workerConcurrency,          // 一次开多少 worker 并发处理任务（本进程内部的 goroutine 个数）；
		Queues:      map[string]int{"discover": 10}, // 哪些队列，优先级是多少（值越大优先级越高）
	})

	// 5) 创建任务路由器（ServeMux）
	// ServeMux 的作用：根据任务类型（Type）找到对应的处理函数
	mux := asynq.NewServeMux()

	// 注册一个处理函数：
	// 只要 Redis 队列里来了 Type 为 "discover:run" 的任务，就会由下面这个 handler 来处理
	mux.HandleFunc("discover:run", func(ctx context.Context, task *asynq.Task) error {
		// -- 5.1 解析任务载荷（payload）--
		// 队列层（queue 包）在入队时会把 DiscoverPayload 编码成 JSON 存在任务里。
		var payload queue.DiscoverPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		// -- 5.2 幂等性保障：若 JobRun 记录还没创建，则补一条 Enqueued --

		// 这里做了一个幂等保护：
		// - 正常情况下，调度器（Scheduler / PTM）在“入队之前”已经调用过 CreateEnqueued；
		// - 若因为某些原因漏掉了（比如调度器 crash），这里 worker 在真正执行前补一条
		exists, err := jobRunRepo.Exists(ctx, payload.RunID)
		if err != nil {
			return err
		}
		if !exists {
			// 若不存在该 run_id 的记录，则以当前时间写一条“已入队”台账
			if err := jobRunRepo.CreateEnqueued(ctx, payload.ScheduleID, payload.RunID, time.Now()); err != nil {
				return err
			}
		}

		// -- 5.3 把 JobRun 状态标记为“已开始”--
		// 记录任务什么时候真正进入执行阶段（可用于观测等待时间 / 队列时延）
		if err := jobRunRepo.MarkStarted(ctx, payload.RunID, time.Now()); err != nil {
			return err
		}

		// -- 5.4 加载规则（Rule）--
		// 一个 RunID 对应一个 RuleID，Rule 定义了：要扫哪些网段、执行配置等
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

		// -- 5.5 根据规则 + 增量/全量标志，选出本次要扫描的目标 IP 列表 --

		// TargetsForRule 会在 DBTargetProvider 里实现的：
		// - 读取 Rule 中配置的目标范围（CIDR / IP 段 / 单 IP）；
		// - 如果 payload.Incremental == true：
		//      只返回“没见过的 + 已经过期（最后一次见到时间 < now - incrementalTTL）”的设备；
		//   否则：
		//      返回所有目标（全量扫描）
		targetsList, err := provider.TargetsForRule(ctx, payload.RuleID, payload.Incremental)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}

		// -- 5.6 加载执行配置：超时、并发数等 --

		// LoadExecutionConfig 从 DiscoveryRuleModel 读取执行参数：
		// - hostTimeoutSecs：每个 ICMP 探测的超时时长（秒）；
		// - concurrency：本次扫描内部的并发 worker 数
		timeoutSecs, concurrency, err := ruleRepo.LoadExecutionConfig(ctx, payload.RuleID)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}

		// -- 5.7 调用领域服务 DiscoveryService 执行真正的「发现」逻辑 --

		start := time.Now()

		// Discover 负责：
		// - 按 concurrency 开几个 goroutine 并发扫描；
		// - 对每个目标发 ICMP，等响应 / 超时；
		// - 调用 ident(识别服务) 解析出设备类型/厂商信息；
		// - 通过 repoDevice 把结果写入 devices 表；
		// - 返回扫描结果的 DTO 列表（可以用于统计）
		dtos, err := discoverySvc.Discover(ctx, rule, targetsList, time.Duration(timeoutSecs)*time.Second, concurrency)
		if err != nil {
			jobRunRepo.MarkFailed(ctx, payload.RunID, time.Now(), err.Error())
			return err
		}
		// 扫描耗时：用于观测性能
		duration := time.Since(start)
		// 扫描到多少条结果（成功探测到的目标数）
		count := len(dtos)

		// -- 5.8 记录本次 JobRun 成功,落库统计信息 --

		// MarkSucceeded 一般会记录：
		// - 结束时间
		// - 总耗时
		// - 成功数量
		// - 还有其它统计字段（看 JobRunModel 定义）
		if err := jobRunRepo.MarkSucceeded(ctx, payload.RunID, time.Now(), duration, count); err != nil {
			return err
		}
		return nil
	})

	// 6) 启动 Worker（异步 goroutine 中运行）
	// srv.Run(mux) 会阻塞当前 goroutine，所以这里用 go 开了一个新协程跑 worker
	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("worker stopped: %v", err)
		}
	}()

	// 7) 主 goroutine 里等待系统信号，实现优雅退出
	//    - 创建一个带缓冲的 channel，接收 OS 信号
	sigCh := make(chan os.Signal, 1)
	//    - 注册要监听的信号：SIGTERM（终止）、SIGINT（Ctrl+C）
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	//    - 阻塞在这里，直到接收到信号
	<-sigCh
	// 一旦收到信号，就调用 srv.Shutdown() 优雅关闭 worker：
	// - 停止接收新任务；
	// - 等待当前正在处理的任务完成；
	// - 关闭相关资源。
	srv.Shutdown()
}

// ---- 配置结构 & 加载配置 ----

// config 用来承接所有需要的运行配置，方便在 main 和 辅助函数之间传递
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

// getIntEnv 读取整型环境变量
// - 若环境变量存在且能成功解析为 int，就用环境变量的值；
// - 否则打印 warning 日志，使用默认值 def
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

// getDurationEnv 读取一个 time.Duration 类型的环境变量。
// - 内部使用 time.ParseDuration，比如 "1h", "30m", "10s" 这种写法。
// - 若环境变量不存在或解析失败，则返回默认值 def
func getDurationEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("invalid duration for %s=%q,use default %s", key, v, def)
	}
	return def
}
