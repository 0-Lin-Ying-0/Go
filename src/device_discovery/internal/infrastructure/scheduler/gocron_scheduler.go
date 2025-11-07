package scheduler

import (
	"context"
	"device_discovery/internal/infrastructure/persistence"
	"device_discovery/internal/infrastructure/persistence/repo"
	"device_discovery/internal/infrastructure/queue"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/teambition/rrule-go"
)

/*
它是“计划调度的执行器/桥接器”：从 DB 读计划 → 在内存里排好钟点 → 到点时把 “扫描任务”安全、可追踪地送进队列，
并维护运行元数据与下一次时间；同时处理时区、重叠策略和增量/全量切换。
*/

type GocronScheduler struct {
	scheduler gocron.Scheduler         // gocron 的核心调度器（接口，便于 mock/替换）
	schedules *repo.ScheduleRepository // 访问 schedule 表：加载启用计划、更新运行时间
	jobRuns   *repo.JobRunRepository   // 访问 job_runs 表：入队记账、统计运行次数
	queue     *queue.Client            // 任务入队(asynq)的封装
	logger    *slog.Logger             // 结构化日志器
}

func NewGocronScheduler(
	dbSchedules *repo.ScheduleRepository,
	jobRuns *repo.JobRunRepository,
	q *queue.Client,
	logger *slog.Logger,
) (*GocronScheduler, error) {
	// 创建 gocron 调度器
	sched, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		// 当外面没传日志器时，给一个默认的结构化日志器
		logger = slog.Default()
	}
	return &GocronScheduler{
		scheduler: sched,
		// dbSchedules/jobRuns：调度要读计划、写运行记录；都在仓储里
		schedules: dbSchedules,
		jobRuns:   jobRuns,
		// 调度只负责“到点触发并入队”
		queue:  q,
		logger: logger,
	}, nil
}

// Start 负责把数据库中的启用计划加载出来，逐一注册到 gocron，然后启动调度器
// 启动流程分三步：查库 → 逐条注册 → 启动，容错友好、并发安全（闭包拷贝）、行为清晰
func (g *GocronScheduler) Start(ctx context.Context) error {
	// 到数据库把所有启用的计划拉出来；失败则直接返回，让上层知道“调度器没加载成功”
	rows, err := g.schedules.ListEnabled(ctx) // 从表里查出 enabled=true 的所有计划
	if err != nil {
		return fmt.Errorf("load schedules: %w", err)
	}
	// 逐条注册
	for _, row := range rows {
		schedule := row
		if err := g.registerSchedule(ctx, schedule); err != nil {
			g.logger.Error("register schedule failed",
				slog.Int64("schedule", row.ID),
				slog.String("err", err.Error()))
		}
	}
	// 启动 gocron,此调用非阻塞：程序会继续往下执行
	g.scheduler.Start() // 启动 gocron 的内部定时器与任务分发
	return nil
}

// registerSchedule 把一条具体的调度记录（row）按它声明的类型与时区，注册成实际会触发的 gocron 任务。
func (g *GocronScheduler) registerSchedule(ctx context.Context, row persistence.DiscoveryScheduleModel) error {
	// 从 IANA 时区数据库加载时区对象（如 "Asia/Taipei"），后续所有“触发时间”“记录时间”都按此时区计算
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone: %w", err)
	}
	switch row.Type {
	case "cron", "interval":
		return g.registerRecurring(ctx, row, loc)
	case "rrule":
		return g.registerRRule(ctx, row, loc)
	default:
		return fmt.Errorf("unsupported type %s", row.Type)
	}
}

// registerRecurring 用来把一条 周期类 调度（cron 或 interval）注册到 gocron
func (g *GocronScheduler) registerRecurring(ctx context.Context, row persistence.DiscoveryScheduleModel, loc *time.Location) error {
	schedule := row
	// OverlapPolicy 生成 Job 选项
	opts := g.buildJobOptions(schedule, loc)
	var job gocron.Job
	// job，让 jobFn 能拿到这个 job 的句柄（用于 NextRun()）
	jobFn := func(ctx context.Context) {
		// 把真正的业务触发逻辑交给 runSchedule：它会记账→入队→更新下一次时间
		g.runSchedule(ctx, schedule, loc, job)
	}
	var def gocron.JobDefinition
	var err error
	switch schedule.Type {
	case "cron":
		// 按 Cron 表达式创建任务定义（false 表示不启用“秒”字段的 5 段 cron）
		def = gocron.CronJob(schedule.Expression, false)
		// interval：把字符串解析成 Duration，如 "5m"、"2h15m"；解析失败直接返回
	case "interval":
		var dur time.Duration
		dur, err = time.ParseDuration(schedule.Expression)
		if err != nil {
			// 按固定间隔循环的任务定义
			def = gocron.DurationJob(dur)
		}
	}
	if err != nil {
		return err
	}
	// 真正注册任务。此时闭包里的 job 已被赋值，等到了触发时 jobFn 能正确拿到 job.NextRun()
	job, err = g.scheduler.NewJob(def, gocron.NewTask(jobFn), opts...)
	if err != nil {
		return err
	}

	// 让 DB 里同步显示“下一次触发时间”
	if next, err := job.NextRun(); err == nil {
		if err := g.schedules.SetNextRun(ctx, schedule.ID, &next); err != nil {
			g.logger.Warn("set next run failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
		}
	}
	return nil
}

/*
	把数据库中 DiscoveryScheduleModel.OverlapPolicy（表里配置的“重叠策略”）翻译成 gocron 的 JobOption，

保证同一个 schedule 同时只能跑一个实例，并定义“触发与正在执行冲突时”的行为
*/
func (g *GocronScheduler) buildJobOptions(row persistence.DiscoveryScheduleModel, loc *time.Location) []gocron.JobOption {
	var opts []gocron.JobOption
	switch row.OverlapPolicy {
	case "queue":
		// 单例 + 等待：不并发，同一计划上一轮没结束则这一轮排队等
		opts = append(opts, gocron.WithSingletonMode(gocron.LimitModeWait))
	default:
		// 单例 + 改期（默认）：不并发，如果冲突则跳过本次、保持节奏
		opts = append(opts, gocron.WithSingletonMode(gocron.LimitModeReschedule))
	}
	return opts
}

// runSchedule 当某条计划被 gocron 触发时，负责把这次触发固化为一次“运行”：到点 → 记账 → 入队 → 回写上下次时间
func (g *GocronScheduler) runSchedule(ctx context.Context, row persistence.DiscoveryScheduleModel, loc *time.Location, job gocron.Job) {
	fire := time.Now().In(loc)                         // 按计划的时区取“触发时间”（记账用）
	runID := fmt.Sprintf("%d:%d", row.ID, fire.Unix()) // 构造 RunID（同一计划同一秒只会有一个 RunID）
	payload := queue.DiscoverPayload{
		ScheduleID:  row.ID,
		RunID:       runID,
		RuleID:      row.RuleID,
		Incremental: shouldRunIncremental(ctx, g.jobRuns, row),
	}
	// 在 job_runs 表插入 ENQUEUED 记录（幂等锚点）,先落库再入队：是实现“至少一次且幂等”的关键
	if err := g.jobRuns.CreateEnqueued(ctx, row.ID, runID, fire); err != nil {
		g.logger.Error("record enqueue failed", slog.String("run_id", runID), slog.String("err", err.Error()))
		return
	}
	/*
		入队失败：把刚才那条 ENQUEUED 改成 FAILED，避免“只记账不执行”的脏状态
		queue.EnqueueDiscover() 把“发现任务”交给 asynq（真正扫描由 worker 执行）
	*/
	if _, err := g.queue.EnqueueDiscover(row.ID, fire.Unix(), payload); err != nil {
		g.logger.Error("enqueue discover task failed", slog.String("run_id", runID), slog.String("err", err.Error()))
		// 入队失败时把这次运行标记为 FAILED
		_ = g.jobRuns.MarkFailed(ctx, runID, time.Now().In(loc), err.Error())
		return
	}
	/*
		成功入队后，更新 LastRunAt=fire；
		若能拿到 NextRun（周期任务、或 one-time 仍可能返回值）就写入；拿不到就写 nil
		这里“job==nil”是为将来手动触发等场景预留；你当前的调用基本都传了 job
	*/
	if job != nil {
		// job.NextRun()：由 gocron 计算的下次触发时间；用于回写 NextRunAt
		if nextRun, err := job.NextRun(); err == nil {
			// schedules.UpdateRunTimes(ctx, row.ID, fire, &nextRun)：把 LastRunAt=fire、NextRunAt=nextRun 回写 DB
			_ = g.schedules.UpdateRunTimes(ctx, row.ID, fire, &nextRun)
		}
	} else {
		_ = g.schedules.UpdateRunTimes(ctx, row.ID, fire, nil)
	}
}

// shouldRunIncremental 就是决定“这次跑 增量 还是 全量”
func shouldRunIncremental(ctx context.Context, jobRuns *repo.JobRunRepository, row persistence.DiscoveryScheduleModel) bool {
	if !row.Incremental {
		return false // 关闭增量 -> 永远全量
	}
	// row.FullEveryNRuns *int：每 N 次做一次全量（指针：nil 表示未配置）
	if row.FullEveryNRuns == nil || *row.FullEveryNRuns <= 0 {
		return true // 没配 N 或 N<=0 -> 每次都增量
	}
	// jobRuns.CountForSchedule(ctx, row.ID)：查这条 schedule 以往的运行次数（通常包含全部状态）
	count, err := jobRuns.CountForSchedule(ctx, row.ID)
	// 失败容错：入队不被阻塞，宁可这次先增量跑掉
	if err != nil {
		return true // 计数查不到 -> 保守选增量(不中断任务)
	}
	if count == 0 {
		return false // 首次运行 -> 全量，建立基线
	}
	// 取模:用来判定“到了该全量的节点”
	if count%int64(*row.FullEveryNRuns) == 0 {
		return false // 每 N 次“之后”的下一次 -> 全量
	}
	return true // 其它情况 -> 增量
}

/*
registerRRule 用来把基于 iCal RRULE 语法（如“每月最后一个工作日 18:00”）的计划注册到调度器里
和 cron/interval 不同，它不是一次性把“所有重复”交给 gocron，而是先算出“下一次发生时间”，然后注册一个一次性任务，等这次任务触发时再递归计算下一次并继续注册。
这样就能忠实还原 RRULE 的复杂规则，又能很好地控制“重叠策略、时区、记账逻辑”
*/
func (g *GocronScheduler) registerRRule(ctx context.Context, row persistence.DiscoveryScheduleModel, loc *time.Location) error {
	schedule := row
	// rrule.StrToRRule(expr)：把字符串表达式解析为 *rrule.RRule 对象
	rule, err := rrule.StrToRRule(schedule.Expression)
	if err != nil {
		return err
	}
	// 以计划时区取当前时间作为“参照基点”
	now := time.Now().In(loc)
	// rule.DTStart(t)：设置 RRULE 的基准起点（DTSTART）。如果表达式里没带 DTSTART，这一步明确从 now 开始推
	rule.DTStart(now)
	// rule.After(t, inc bool)：给定时间点 t，返回下一次发生时间；inc=false 表示不包含 t 本身（严格晚于 t）
	next := rule.After(now.Add(-time.Second), false)
	// next.IsZero() 意味着规则永不再发生（比如 COUNT 已经耗尽，或 UNTIL 已过）；直接报错结束
	if next.IsZero() {
		return fmt.Errorf("rrule produced no occurrence")
	}
	return g.scheduleRRuleOccurrence(ctx, schedule, loc, rule, next)
}

// 把已经算出来的下一次 RRULE 触发时间 runAt
// 内部封装 —— 把“计算出的下一次时间”注册成一个 one-time 任务，并在 Job 里递归注册后续发生时间。从而形成“RRULE 严格重复、每次只占一个定时器”的链式调度
func (g *GocronScheduler) scheduleRRuleOccurrence(ctx context.Context, row persistence.DiscoveryScheduleModel, loc *time.Location, rule *rrule.RRule, runAt time.Time) error {
	schedule := row
	// g.buildJobOptions(schedule, loc)：把 OverlapPolicy 翻译成 gocron 的 Singleton 模式（Wait/Reschedule），防止同一计划重叠执行
	opts := g.buildJobOptions(schedule, loc)
	runAt = runAt.In(loc) //确保 NextRunAt 与实际触发按照同一时区记录
	// g.schedules.SetNextRun(ctx, schedule.ID, &runAt)：把这次算出来的“下一次触发时刻”写回 DB
	if err := g.schedules.SetNextRun(ctx, schedule.ID, &runAt); err != nil {
		g.logger.Warn("set next run failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
	}
	// 触发时先执行“这次”;再立即计算/注册下一次 one-time
	var job gocron.Job
	// 真正触发时要做的事
	jobFn := func(ctx context.Context) {
		g.runSchedule(ctx, schedule, loc, job) //入队并记账
		// 返回严格晚于 t 的下一次 occurrence（false 表示不包含 t 本身）
		// rule.After(runAt, false) 计算下一次，再调用 scheduleRRuleOccurrence(...) 注册下一个 one-time
		next := rule.After(runAt, false)
		if !next.IsZero() {
			// context.Background() 递归注册（意味着不受当前 ctx 取消影响，保证能把下一次接力上）
			if err := g.scheduleRRuleOccurrence(context.Background(), schedule, loc, rule, next); err != nil {
				g.logger.Error("schedule next rrule occurrence failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
			}
		}
	}

	// 真正把“在 runAt 执行 jobFn 的 one-time 任务”注册进调度器
	var err error
	job, err = g.scheduler.NewJob(
		// 定义一个只会触发一次的 gocron 任务，时间点为 runAt
		gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(runAt)),
		gocron.NewTask(jobFn),
		opts...,
	)
	if err != nil {
		return err
	}
	return nil
}

// Stop 不再触发新的任务，让正在跑的任务按自己的逻辑跑完（或自行取消），随后释放调度线程
func (g *GocronScheduler) Stop() error {
	if g.scheduler == nil {
		return nil
	}
	// 停止 gocron。它会停止后续触发；已在执行中的 job 不会被库强杀（Go 里没法强制终止 goroutine），要靠你的任务函数通过 context.Context 自行感知取消并尽快返回
	return g.scheduler.Shutdown()
}
