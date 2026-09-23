package ping

import (
	"context"
	"errors"
	"sort"
	"time"

	nodefeature "github.com/sonar-probe/sonar/internal/features/node"
	"github.com/sonar-probe/sonar/internal/platform/dbcore"
	"github.com/sonar-probe/sonar/internal/platform/metricruntime"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/pingpresets"
	"gorm.io/gorm"
)

// builtinPresetInterval 是内置省市三网节点的固定探测间隔（秒）。
// 持续监控走完整 TCP 握手，礼貌、低频，跟手动大包测试是两套完全独立的机制。
const builtinPresetInterval = 60

func init() {
	nodefeature.SetPingResultRecorder(SavePingRecord)
	nodefeature.SetDefaultPingTaskApplier(AddDefaultOnClientUUID)
}

// AddPingTask 创建延迟监测任务。defaultOn 表示新加入的服务器是否自动开启此监测。
func AddPingTask(clients []string, defaultOn bool, name string, target, taskType string, interval int) (uint, error) {
	db := dbcore.GetDBInstance()
	normalizedClients := normalizePingClients(models.StringArray(clients))
	task := models.PingTask{
		Clients:   normalizedClients,
		DefaultOn: defaultOn,
		Name:      name,
		Type:      taskType,
		Target:    target,
		Interval:  interval,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}

		// Append by id to avoid races between concurrent create requests.
		result := tx.Model(&models.PingTask{}).Where("id = ?", task.ID).Update("weight", int(task.ID))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return task.ID, ReloadPingSchedule()
}

func DeletePingTask(id []uint) error {
	// The metric store is independent from the main database, so clean it first
	// to avoid leaving history that can no longer be addressed through the task.
	if err := DeletePingRecords(id); err != nil {
		return err
	}

	db := dbcore.GetDBInstance()
	result := db.Where("id IN ?", id).Delete(&models.PingTask{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return ReloadPingSchedule()
}

// EditPingTask 批量更新延迟监测任务配置。
func EditPingTask(tasks []*models.PingTask) error {
	db := dbcore.GetDBInstance()
	for _, task := range tasks {
		task.Clients = normalizePingClients(task.Clients)
		// 使用 map 显式更新，避免 GORM struct Updates 跳过 false/0/空切片等零值。
		updates := map[string]any{
			"name":        task.Name,
			"clients":     task.Clients,
			"all_clients": task.DefaultOn,
			"type":        task.Type,
			"target":      task.Target,
			"interval":    task.Interval,
		}
		result := db.Model(&models.PingTask{}).Where("id = ?", task.ID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return ReloadPingSchedule()
}

// SyncClientPingNodes 全量同步"这台服务器要监测哪些节点"：内置节点按
// (省份,运营商) 精确匹配 target 判断，自建任务按 ID 判断。选中的要确保这台
// 服务器在 Clients 里（内置节点不存在对应 PingTask 就新建）；没选中但当前
// 绑着这台服务器的，就把它从 Clients 里移除——不删任务本身，因为可能还
// 绑着别的服务器。builtinInterval 是这次勾选的内置节点统一使用的间隔，
// 会覆盖这些任务原有的 interval（哪怕是别的服务器之前设置的）。
func SyncClientPingNodes(clientUUID string, selectedBuiltin []pingpresets.Node, builtinInterval int, selectedCustomTaskIDs []uint) error {
	if clientUUID == "" {
		return errors.New("client is required")
	}
	if builtinInterval <= 0 {
		builtinInterval = builtinPresetInterval
	}

	builtinTargetWanted := make(map[string]bool, len(selectedBuiltin))
	builtinTargetToNode := make(map[string]pingpresets.Node, len(selectedBuiltin))
	for _, n := range selectedBuiltin {
		builtinTargetWanted[n.Target] = true
		builtinTargetToNode[n.Target] = n
	}
	customWanted := make(map[uint]bool, len(selectedCustomTaskIDs))
	for _, id := range selectedCustomTaskIDs {
		customWanted[id] = true
	}
	allBuiltinTargets := make(map[string]bool, len(pingpresets.Nodes(4)))
	for _, n := range pingpresets.Nodes(4) {
		allBuiltinTargets[n.Target] = true
	}

	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error {
		var tasks []models.PingTask
		if err := tx.Find(&tasks).Error; err != nil {
			return err
		}

		seenBuiltinTargets := make(map[string]bool, len(selectedBuiltin))
		for _, task := range tasks {
			isBuiltin := allBuiltinTargets[task.Target]
			var want bool
			if isBuiltin {
				want = builtinTargetWanted[task.Target]
				if want {
					seenBuiltinTargets[task.Target] = true
				}
			} else {
				want = customWanted[task.ID]
			}

			has := clientInList(task.Clients, clientUUID)
			nextClients := task.Clients
			if want && !has {
				nextClients = append(append(models.StringArray{}, task.Clients...), clientUUID)
			} else if !want && has {
				nextClients = removeClientFromList(task.Clients, clientUUID)
			}

			updates := map[string]any{}
			if want != has {
				updates["clients"] = normalizePingClients(nextClients)
			}
			if isBuiltin && want {
				updates["interval"] = builtinInterval
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&models.PingTask{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
				return err
			}
		}

		// 选中的内置节点里，还没有对应 PingTask 的，新建一条。
		for _, node := range selectedBuiltin {
			if seenBuiltinTargets[node.Target] {
				continue
			}
			newTask := models.PingTask{
				Clients:  models.StringArray{clientUUID},
				Name:     node.Name,
				Type:     "tcp",
				Target:   node.Target,
				Interval: builtinInterval,
			}
			if err := tx.Create(&newTask).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.PingTask{}).Where("id = ?", newTask.ID).Update("weight", int(newTask.ID)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ReloadPingSchedule()
}

func clientInList(list models.StringArray, uuid string) bool {
	for _, c := range list {
		if c == uuid {
			return true
		}
	}
	return false
}

func removeClientFromList(list models.StringArray, uuid string) models.StringArray {
	next := make(models.StringArray, 0, len(list))
	for _, c := range list {
		if c != uuid {
			next = append(next, c)
		}
	}
	return next
}

// normalizePingClients 保持 clients 字段序列化为 JSON 数组，避免空值变成 null。
func normalizePingClients(clients models.StringArray) models.StringArray {
	if clients == nil {
		return models.StringArray{}
	}
	return clients
}

func GetAllPingTasks() ([]models.PingTask, error) {
	db := dbcore.GetDBInstance()
	var tasks []models.PingTask
	if err := db.Order("weight ASC").Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetPingTasksByClient 获取指定服务器需要执行的延迟监测任务。
func GetPingTasksByClient(uuid string) []models.PingTask {
	db := dbcore.GetDBInstance()
	var tasks []models.PingTask
	if err := db.Where("clients LIKE ?", `%"`+uuid+`"%`).Order("weight ASC").Order("id ASC").Find(&tasks).Error; err != nil {
		return nil
	}
	return tasks
}

func UpdatePingTaskOrder(order map[uint]int) error {
	if len(order) == 0 {
		return nil
	}

	db := dbcore.GetDBInstance()
	err := db.Transaction(func(tx *gorm.DB) error {
		// Validate all ids before changing any weights. The update response is
		// not a reliable existence check because some drivers report zero rows
		// when the new value equals the current value.
		ids := make([]uint, 0, len(order))
		for id := range order {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		var existing int64
		if err := tx.Model(&models.PingTask{}).Where("id IN ?", ids).Count(&existing).Error; err != nil {
			return err
		}
		if existing != int64(len(ids)) {
			return gorm.ErrRecordNotFound
		}

		for _, id := range ids {
			weight := order[id]
			result := tx.Model(&models.PingTask{}).Where("id = ?", id).Update("weight", weight)
			if result.Error != nil {
				return result.Error
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ReloadPingSchedule()
}

// ping 记录已完全迁移到 metric store（指标 ping.latency_ms），运行期读写全部走
// metric store，旧 ping_records 表不再参与。

func SavePingRecord(record models.PingRecord) error {
	return metricruntime.WritePingRecord(context.Background(), record)
}

func DeletePingRecords(id []uint) error {
	return metricruntime.DeletePingRecordsByTask(context.Background(), id)
}

func DeleteAllPingRecords() error {
	return metricruntime.DeleteAllPingRecords(context.Background())
}

func ReloadPingSchedule() error {
	pingTasks, err := GetAllPingTasks()
	if err != nil {
		return err
	}
	return pingManager.reload(pingTasks)
}

// AddDefaultOnClientUUID 在新客户端注册后，把该 UUID 追加到所有 default_on=true 的任务的 clients 中（去重）。
func AddDefaultOnClientUUID(uuid string) error {
	if uuid == "" {
		return nil
	}
	db := dbcore.GetDBInstance()
	var tasks []models.PingTask
	if err := db.Where("all_clients = ?", true).Find(&tasks).Error; err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	changed := false
	for _, task := range tasks {
		exists := false
		for _, c := range task.Clients {
			if c == uuid {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		next := append(models.StringArray{}, task.Clients...)
		next = append(next, uuid)
		if err := db.Model(&models.PingTask{}).Where("id = ?", task.ID).Update("clients", next).Error; err != nil {
			return err
		}
		changed = true
	}
	if changed {
		return ReloadPingSchedule()
	}
	return nil
}

func GetPingRecords(uuid string, taskId int, start, end time.Time) ([]models.PingRecord, error) {
	return metricruntime.GetPingRecords(context.Background(), uuid, taskId, start, end)
}
