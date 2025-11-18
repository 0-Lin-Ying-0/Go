package scheduler

import (
	"context"
	"device_discovery/internal/infrastructure/persistence"
	"device_discovery/internal/infrastructure/persistence/repo"
	"device_discovery/internal/infrastructure/queue"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
)

/*
“数据库里的调度表”➜转成 Asynq 的动态周期任务（PTM），再把每一次到点的“调度触发”转译成要跑的“设备发现任务”
*/

const (
	triggerTaskType  = "scheduler:trigger" // 触发器任务的"类型名"(标签)
	triggerQueueName = "scheduler_trigger" // 触发器任务走的专用队列
)

// 触发器 payload —— 只需要知道是哪条 schedule 到点了
type triggerPayload struct {
	ScheduleID int64 `json:"schedule_id"`
}

type AsynqScheduler struct {
	schedules *repo.ScheduleRepository
	jobRuns   *repo.JobRunRepository
	queue     *queue.Client
	logger    *slog.Logger

	redisOpt asynq.RedisClientOpt

	manager *asynq.PeriodicTaskManager
	server  *asynq.Server
	mux     *asynq.ServeMux

	ctx    context.Context
	cancel context.CancelFunc
}

func NewAsynqScheduler(
	redisAddr string,
	schedules *repo.ScheduleRepository,
	jobRuns *repo.JobRunRepository,
	q *queue.Client,
	logger *slog.Logger,
) *AsynqScheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AsynqScheduler{
		schedules: schedules,
		jobRuns:   jobRuns,
		queue:     q,
		logger:    logger,
		redisOpt:  asynq.RedisClientOpt{Addr: redisAddr},
	}
}

func (s *AsynqScheduler) Start(ctx context.Context) error {
	// 依赖完整性自检
	if s.schedules == nil || s.jobRuns == nil || s.queue == nil {
		return fmt.Errorf("scheduler not properly initialized")
	}

	// 统一取消
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 把 DB 中启用的调度行 -> PeriodicTaskConfig
	provider := &scheduleConfigProvider{
		schedules: s.schedules,
		logger:    s.logger,
	}

	if err := s.refreshNextRuns(s.ctx); err != nil {
		s.logger.Warn("refresh next runs failed", slog.String("err", err.Error()))
	}

	/*
	 创建动态周期任务管理器 PTM
	      - PeriodicTaskConfigProvider：PTM 定期调用它同步调度条目
	      - SchedulerOpts.PreEnqueueFunc：只读型“入队前”回调（必须快速返回）
	*/
	manager, err := asynq.NewPeriodicTaskManager(
		asynq.PeriodicTaskManagerOpts{
			RedisConnOpt:               s.redisOpt,
			PeriodicTaskConfigProvider: provider,
			SchedulerOpts: &asynq.SchedulerOpts{
				PreEnqueueFunc: s.preEnqueue, // 仅通知，无返回值，不能中止入队
			},
		})
	if err != nil {
		return fmt.Errorf("create periodic task manager: %w", err)
	}
	s.manager = manager

	// “触发器队列”的轻量消费者：把 scheduler:trigger 任务快速吃掉
	// 这样“时间信号”不会在 Redis 里堆积；真正业务派发在 preEnqueue 里完成
	s.server = asynq.NewServer(s.redisOpt, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{triggerQueueName: 1},
	})
	s.mux = asynq.NewServeMux()
	s.mux.HandleFunc(triggerTaskType, func(ctx context.Context, task *asynq.Task) error {
		return nil // 触发器不执行业务
	})
	go func() {
		if err := s.server.Run(s.mux); err != nil {
			s.logger.Error("trigger server stopped", slog.String("err", err.Error()))
		}
	}()

	// 启动 PTM (内部 scheduler + 后台 sync goroutine(同步协程))
	if err := s.manager.Start(); err != nil {
		return fmt.Errorf("start periodic manager: %w", err)
	}
	return nil
}

func (s *AsynqScheduler) Stop() error {
	if s.cancel != nil {
		s.cancel() // 不再有配置同步
	}
	if s.manager != nil {
		s.manager.Shutdown() // 不再有新 trigger
	}
	if s.server != nil {
		s.server.Shutdown() // 不再消耗 trigger 队列
	}
	return nil
}

type scheduleConfigProvider struct {
	schedules *repo.ScheduleRepository
	logger    *slog.Logger
}

// 从数据库里读出“启用中的调度记录”，把每一条转成 asynq 的 PeriodicTaskConfig，交给 PeriodicTaskManager 去调度

func (p *scheduleConfigProvider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	rows, err := p.schedules.ListEnabled(context.Background())
	if err != nil {
		return nil, err
	}
	var configs []*asynq.PeriodicTaskConfig
	for _, row := range rows {
		if row.Type == "rrule" {
			continue
		}

		// 触发器任务：只带 schedule_id；禁用重试
		payload, err := json.Marshal(triggerPayload{ScheduleID: row.ID})
		if err != nil {
			p.logger.Error("marshal trigger payload failed", slog.Int64("schedule", row.ID), slog.String("err", err.Error()))
			continue
		}
		task := asynq.NewTask(triggerTaskType, payload,
			asynq.Queue(triggerQueueName),
			asynq.MaxRetry(0),
		)

		// 生成 cronspec：cron 用 CRON_TZ 前缀；interval 用 @every
		var spec string
		switch row.Type {
		case "cron":
			spec = fmt.Sprintf("CRON_TZ=%s %s", row.Timezone, row.Expression)
		case "interval":
			if _, err := time.ParseDuration(row.Expression); err != nil {
				p.logger.Error("invalid interval expression", slog.Int64("schedule", row.ID), slog.String("err", err.Error()))
				continue
			}
			spec = fmt.Sprintf("@every %s", row.Expression)
		default:
			continue
		}

		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: spec,
			Task:     task,
		})
	}
	return configs, nil
}

// preEnqueue 里做决策与真正派发
func (s *AsynqScheduler) preEnqueue(task *asynq.Task, _ []asynq.Option) {
	// 若整体调度器已取消，快速返回
	if s.ctx != nil {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
	}

	// 解析触发器 payload
	var payload triggerPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		s.logger.Error("decode trigger payload failed", slog.String("err", err.Error()))
		return
	}

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 读最新 schedule; 过滤：未找到/禁用/rrule 直接返回
	schedule, err := s.schedules.FindByID(ctx, payload.ScheduleID)
	if err != nil {
		s.logger.Error("load schedule failed", slog.Int64("schedule", payload.ScheduleID), slog.String("err", err.Error()))
		return
	}
	if schedule == nil || !schedule.Enable {
		return
	}
	if schedule.Type == "rrule" {
		s.logger.Warn("rrule schedule not supported yet", slog.Int64("schedule", schedule.ID))
		return
	}

	// 计算当前触发时间 fire,计算下一次 next_run
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		s.logger.Error("load location failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
		return
	}
	fire := time.Now().In(loc)
	next, err := nextRunTime(*schedule, loc, fire)
	if err != nil {
		s.logger.Error("calculate next run failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
	}

	// 重叠策略：非queue;同一任务上一次还在跑，那这一次调度就直接跳过，但更新 next_run
	if schedule.OverlapPolicy != "queue" {
		active, err := s.jobRuns.HasActiveRun(ctx, schedule.ID)
		if err != nil {
			s.logger.Error("check active run failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
			return
		}
		if !active {
			if next != nil {
				if err := s.schedules.SetNextRun(ctx, schedule.ID, next); err != nil {
					s.logger.Warn("set next run failed", slog.Int64("schedule", schedule.ID), slog.String("err", err.Error()))
				}
			}
			return
		}
	}

	// 真正派发业务：记台账 + 入业务队列 + 回写 last/next
	// 所以 preEnqueue 的角色：
	//“在真正 run 之前的决策 + 时钟维护 + 状态校验”，
	//而所有干活的逻辑都留到 runSchedule 里
	s.runSchedule(ctx, *schedule, loc, fire, next)
}

func nextRunTime(schedule persistence.DiscoveryScheduleModel, loc *time.Location, from time.Time) (*time.Time, error) {
	switch schedule.Type {
	case "cron":
		// 默认五段，不含秒
		// 创建一个解析 cron 表达式的 解析器 Parser
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		// 把字符串形式的 cron 表达式解析成一个 Schedule 对象
		expr, err := parser.Parse(schedule.Expression)
		if err != nil {
			return nil, err
		}
		// 以 from 为基准，算出“下一次匹配这个 cron 表达式的时间点”
		next := expr.Next(from)
		return &next, nil
	case "interval":
		dur, err := time.ParseDuration(schedule.Expression)
		if err != nil {
			return nil, err
		}
		next := from.Add(dur)
		return &next, nil

	default:
		return nil, fmt.Errorf("unsupported schedule type: %s", schedule.Type)
	}
}

// runSchedule：台账 + 入业务队列 + 回写 last/next
func (s *AsynqScheduler) runSchedule(ctx context.Context, row persistence.DiscoveryScheduleModel,
	loc *time.Location, fire time.Time, next *time.Time,
) {
	// 给这次运行生成一个唯一的 runID（方便在 job_runs 表里记账、追踪）
	runID := fmt.Sprintf("%d:%d", row.ID, fire.Unix())

	// 真正发给 worker 的业务 payload
	payload := queue.DiscoverPayload{
		ScheduleID:  row.ID,
		RunID:       runID,
		RuleID:      row.RuleID,
		Incremental: shouldRunIncremental(ctx, s.jobRuns, row),
	}

	// 先记 已入队 (enqueued)
	if err := s.jobRuns.CreateEnqueued(ctx, row.ID, runID, fire); err != nil {
		s.logger.Error("record enqueue failed", slog.String("run_id", runID), slog.String("err", err.Error()))
		return
	}

	// 把真正的"发现任务"入业务队列
	if _, err := s.queue.EnqueueDiscover(row.ID, fire.Unix(), payload); err != nil {
		s.logger.Error("enqueue discover task failed", slog.String("run_id", runID), slog.String("err", err.Error()))
		_ = s.jobRuns.MarkFailed(ctx, runID, time.Now().In(loc), err.Error())
		return
	}

	// 回写 last_run / next_run ,UI 能看到 刚跑的 & 下次何时跑
	if next != nil {
		_ = s.schedules.UpdateRunTimes(ctx, row.ID, fire, next)
	} else {
		_ = s.schedules.UpdateRunTimes(ctx, row.ID, fire, nil)
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

// 启动前“预热” next_run,把"所有启用的调度"的下一次触发时间，按各自的时区重新算一遍,写回数据库
func (s *AsynqScheduler) refreshNextRuns(ctx context.Context) error {
	// 查出启用的调度
	rows, err := s.schedules.ListEnabled(ctx)
	if err != nil {
		return err
	}
	// 遍历每一条调度
	for _, row := range rows {
		if row.Type == "rrule" {
			continue
		}
		// 加载时区 time.LoadLocation
		loc, err := time.LoadLocation(row.Timezone)
		if err != nil {
			s.logger.Warn("load location failed", slog.Int64("schedule", row.ID), slog.String("err", err.Error()))
			continue
		}
		// 获取当前时间
		now := time.Now().In(loc)
		// 调用 nextRunTime 计算下一次执行时间
		next, err := nextRunTime(row, loc, now)
		if err != nil {
			s.logger.Warn("calculate next run failed", slog.Int64("schedule", row.ID), slog.String("err", err.Error()))
			continue
		}
		// next 写回数据库
		if next != nil {
			if err := s.schedules.SetNextRun(ctx, row.ID, next); err != nil {
				s.logger.Warn("set next run failed", slog.Int64("schedule", row.ID), slog.String("err", err.Error()))
			}
		}
	}
	return nil
}
