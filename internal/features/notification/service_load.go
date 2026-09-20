package notification

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/komari-monitor/komari/internal/features/notification/messagesender"
	"github.com/komari-monitor/komari/internal/platform/clients"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/models"
	messageevent "github.com/komari-monitor/komari/internal/platform/models/messageEvent"
	"github.com/komari-monitor/komari/internal/platform/records"
	"github.com/komari-monitor/komari/pkg/logger"
	"github.com/komari-monitor/komari/pkg/scheduler"
)

// LoadNotificationService 管理定时器和任务
type LoadNotificationService struct {
	mu    sync.Mutex
	tasks map[int][]models.LoadNotification
}

var LoadNotificationManager = &LoadNotificationService{
	tasks: make(map[int][]models.LoadNotification),
}

// Reload 重载时间表
func (m *LoadNotificationService) Reload(loadNotifications []models.LoadNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	scheduler.RemovePrefix("load-notification:")
	m.tasks = make(map[int][]models.LoadNotification)

	// 按Interval分组任务
	taskGroups := make(map[int][]models.LoadNotification)
	for _, task := range loadNotifications {
		taskGroups[task.Interval] = append(taskGroups[task.Interval], task)
	}

	// 为每个唯一的Interval创建定时器
	for interval, tasks := range taskGroups {
		interval := interval
		tasks := append([]models.LoadNotification(nil), tasks...)
		m.tasks[interval] = tasks
		if err := scheduler.AddFunc(fmt.Sprintf("load-notification:%d", interval), scheduler.Every(time.Duration(interval)*time.Minute), func() {
			for _, task := range tasks {
				go executeLoadNotificationTask(task)
			}
		}); err != nil {
			return err
		}
	}

	return nil
}

// executeLoadNotificationTask 执行单个LoadNotificationTask
func executeLoadNotificationTask(task models.LoadNotification) {
	// 检查是否在冷却期内
	if shouldSkipNotification(task) {
		return
	}

	now := time.Now().UTC()
	windowStart := now.Add(-time.Duration(task.Interval) * time.Minute)
	overloadClients := make([]string, 0)
	for _, clientUUID := range task.Clients {
		// 仅查询当前通知使用的指标，避免重建完整监控记录。
		records, err := getMetricRecordsForClient(clientUUID, task.Metric, windowStart, now)
		if err != nil {
			continue
		}

		// 检查指标是否达到阈值
		if checkMetricThreshold(records, task) {
			overloadClients = append(overloadClients, clientUUID)
		}

	}
	sendLoadNotification(overloadClients, task)
	updateLastNotified(task.Id, now)
}

// shouldSkipNotification 检查是否应该跳过通知（冷却期检查）
func shouldSkipNotification(task models.LoadNotification) bool {
	if task.LastNotified == nil || task.LastNotified.IsZero() {
		return false
	}

	// 计算冷却期（使用 interval 作为冷却期）
	cooldownPeriod := time.Duration(task.Interval) * time.Minute
	timeSinceLastNotified := time.Since(*task.LastNotified)

	return timeSinceLastNotified < cooldownPeriod
}

// getMetricRecordsForClient 获取指定客户端在时间窗口内的单项指标最大值。
func getMetricRecordsForClient(clientUUID, metricName string, start, end time.Time) ([]models.Record, error) {
	return records.GetRecordMetricMaxByClientAndTime(clientUUID, metricName, start, end)
}

// checkMetricThreshold 检查指标是否达到阈值
func checkMetricThreshold(records []models.Record, task models.LoadNotification) bool {
	if len(records) == 0 {
		return false
	}

	// 计算需要达标的最小记录数
	minRequiredRecords := int(float32(len(records)) * task.Ratio)
	if minRequiredRecords == 0 {
		minRequiredRecords = 1
	}

	exceededCount := 0

	for _, record := range records {
		metricValue := getMetricValue(record, task.Metric)
		if metricValue >= task.Threshold {
			exceededCount++
		}
	}

	return exceededCount >= minRequiredRecords
}

// getMetricValue 根据指标名称获取记录中的对应值
func getMetricValue(record models.Record, metric string) float32 {
	switch metric {
	case "cpu":
		return record.Cpu
	case "gpu":
		return record.Gpu
	case "net_in", "netin":
		return bytesPerSecondToMbps(record.NetIn)
	case "net_out", "netout":
		return bytesPerSecondToMbps(record.NetOut)
	case "ram":
		return usagePercentOfClientTotal(record, record.Ram, func(c models.Client) int64 { return c.MemTotal })
	case "swap":
		return usagePercentOfClientTotal(record, record.Swap, func(c models.Client) int64 { return c.SwapTotal })
	case "load":
		return record.Load
	case "temp":
		return record.Temp
	case "disk":
		return usagePercentOfClientTotal(record, record.Disk, func(c models.Client) int64 { return c.DiskTotal })
	default:
		// 尝试通过反射获取字段值
		v := reflect.ValueOf(record)
		field := v.FieldByName(metric)
		if field.IsValid() && field.CanInterface() {
			switch field.Kind() {
			case reflect.Float32:
				return float32(field.Float())
			case reflect.Float64:
				return float32(field.Float())
			case reflect.Int, reflect.Int32, reflect.Int64:
				return float32(field.Int())
			}
		}
		return 0
	}
}

// usagePercentOfClientTotal 用客户端总量字段（如 MemTotal/SwapTotal/DiskTotal）
// 把一个绝对使用量换算成百分比；查不到客户端或总量为 0 时返回 0。
func usagePercentOfClientTotal(record models.Record, used int64, total func(models.Client) int64) float32 {
	client, err := clients.GetClientByUUID(record.Client)
	if err != nil {
		logger.Errorf("notifier", "Failed to get client info for %s: %v", record.Client, err)
		return 0
	}
	clientTotal := total(client)
	if clientTotal <= 0 {
		return 0
	}
	return float32(used) / float32(clientTotal) * 100
}

func bytesPerSecondToMbps(bytesPerSecond int64) float32 {
	if bytesPerSecond <= 0 {
		return 0
	}

	// 采用十进制 Mbps：1 Mbps = 1,000,000 bit/s
	return float32(float64(bytesPerSecond) * 8 / 1_000_000)
}

// sendLoadNotification 发送负载通知
func sendLoadNotification(clientUUIDs []string, task models.LoadNotification) {
	if len(clientUUIDs) == 0 {
		return
	}
	eventClients := make([]models.Client, 0, len(clientUUIDs))
	for _, uuid := range clientUUIDs {
		eventClients = append(eventClients, models.Client{UUID: uuid})
	}
	go func() {
		if err := messagesender.SendNotification(models.EventMessage{
			Event:   messageevent.Alert,
			Clients: eventClients,
			Time:    time.Now().UTC(),
			Emoji:   "⚠️",
			Message: task.Name,
		}); err != nil {
			logger.Errorf("notifier", "Failed to send load notification for task %d: %v", task.Id, err)
		}
	}()
}

// updateLastNotified 更新最后通知时间
func updateLastNotified(taskId uint, notifyTime time.Time) {
	db := dbcore.GetDBInstance()
	if err := db.Model(&models.LoadNotification{}).Where("id = ?", taskId).Update("last_notified", notifyTime.UTC()).Error; err != nil {
		logger.Errorf("notifier", "Failed to update last_notified for task %d: %v", taskId, err)
	}
}

// reloadLoadNotificationSchedule 加载或重载时间表。导出的入口是
// store_load.go 里的 ReloadLoadNotificationSchedule()（无参，从数据库取数据）。
func reloadLoadNotificationSchedule(loadNotifications []models.LoadNotification) error {
	return LoadNotificationManager.Reload(loadNotifications)
}
