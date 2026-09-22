package ping

import (
	"context"
	"fmt"
	"sync"
	"time"

	nodefeature "github.com/sonar-probe/sonar/internal/features/node"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/protocol"
	"github.com/sonar-probe/sonar/pkg/scheduler"
)

// pingTaskManager 管理定时器和任务
type pingTaskManager struct {
	mu    sync.Mutex
	tasks map[int][]models.PingTask
}

var pingManager = &pingTaskManager{
	tasks: make(map[int][]models.PingTask),
}

// reload 重载时间表
func (m *pingTaskManager) reload(pingTasks []models.PingTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	scheduler.RemovePrefix("ping:")
	m.tasks = make(map[int][]models.PingTask)

	// 按Interval分组任务
	taskGroups := make(map[int][]models.PingTask)
	for _, task := range pingTasks {
		if task.Interval <= 0 {
			continue
		}
		taskGroups[task.Interval] = append(taskGroups[task.Interval], task)
	}

	// 为每个唯一的Interval创建协程
	for interval, tasks := range taskGroups {
		interval := interval
		tasks := append([]models.PingTask(nil), tasks...)
		m.tasks[interval] = tasks
		if err := scheduler.AddContextFunc(fmt.Sprintf("ping:%d", interval), scheduler.Every(time.Duration(interval)*time.Second), false, func(ctx context.Context) {
			for _, task := range tasks {
				go executePingTask(ctx, task)
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// executePingTask 执行单个PingTask
func executePingTask(ctx context.Context, task models.PingTask) {
	for _, clientUUID := range targetPingClientUUIDs(task) {
		select {
		case <-ctx.Done():
			// Context was canceled, stop sending pings.
			return
		default:
			// Context is still active, continue.
		}

		nodefeature.DispatchPing(clientUUID, protocol.PingParams{TaskID: task.ID, Type: task.Type, Target: task.Target})
	}
}

// targetPingClientUUIDs 返回任务配置的目标服务器列表；在线与否的判断留给
// DispatchPing（离线客户端会被静默跳过），这里不做过滤。
func targetPingClientUUIDs(task models.PingTask) []string {
	return task.Clients
}
