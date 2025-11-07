package queue

/*
把“调度触发”真正丢进任务队列:为“设备发现”这一业务，包装 asynq.Client，提供一个领域化的队列投递入口
当调度器某个时间点触发时，它不会自己去扫网段，而是把任务塞进队列，由独立的 Worker 异步执行
*/

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// 队列客户端适配器,把第三方库 asynq.Client 包起来，形成项目里的 queue.Client，方便在上层只依赖自己的接口/类型

type Client struct {
	client *asynq.Client
}

func New(addr string) *Client {
	// 创建底层 asynq 客户端。RedisClientOpt 是连接配置的复合字面量，这里只设了 Addr
	c := asynq.NewClient(asynq.RedisClientOpt{Addr: addr})
	return &Client{client: c}
}

// Close 统一、干净地释放队列客户端资源
func (c *Client) Close() error {
	return c.client.Close()
}

// DiscoverPayload 发送给 Worker 队列的消息
type DiscoverPayload struct {
	ScheduleID  int64  `json:"schedule_id"`
	RunID       string `json:"run_id"`
	RuleID      int64  `json:"rule_id"`
	Incremental bool   `json:"incremental"`
}

// EnqueueDiscover 把一次“调度触发”包装成 asynq 任务 并安全入队
func (c *Client) EnqueueDiscover(scheduleID int64, fireUnix int64, payload DiscoverPayload) (*asynq.TaskInfo, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// 定义任务类型。Worker 端会按 "discover:run" 注册处理器
	task := asynq.NewTask("discover:run", body)
	// 幂等关键点：同一条 scheduleID、同一触发秒 fireUnix → 相同 TaskID
	taskID := fmt.Sprintf("sch-%d-%d", scheduleID, fireUnix)
	// 入队 + 运行策略
	return c.client.Enqueue(task,
		// 显式任务 ID → 避免重复投递
		asynq.TaskID(taskID),
		// 投到指定队列，便于与其它业务隔离
		asynq.Queue("discover"),
		// 任务完成后在 Redis 中保留 10 分钟，便于排错/观察
		asynq.Retention(10*time.Minute),
		// Worker 侧执行超时保护
		asynq.Timeout(15*time.Minute),
		// 失败自动重试上限（带退避）
		asynq.MaxRetry(3),
	)
}
