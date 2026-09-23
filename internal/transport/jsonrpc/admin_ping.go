package jsonrpc

import (
	"github.com/sonar-probe/sonar/internal/features/ping"
	"github.com/sonar-probe/sonar/pkg/rpc"
)

// admin_ping.go
// 延迟监测任务（ping task）RPC2 方法注册（admin 命名空间）。
// 处理逻辑在 internal/features/ping，这里只做注册，
// 因为注册中心（RegisterWithGroupAndMeta）就在本包。

func init() {
	RegisterWithGroupAndMeta("addPingTask", rpc.RoleAdmin, ping.AdminAddTask, &rpc.MethodMeta{
		Name:    "admin:addPingTask",
		Summary: "Create a ping task",
		Returns: "{ task_id: uint }",
	})
	RegisterWithGroupAndMeta("deletePingTask", rpc.RoleAdmin, ping.AdminDeleteTask, &rpc.MethodMeta{
		Name:    "admin:deletePingTask",
		Summary: "Delete ping tasks by ids",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("editPingTask", rpc.RoleAdmin, ping.AdminEditTask, &rpc.MethodMeta{
		Name:    "admin:editPingTask",
		Summary: "Edit ping tasks",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("getAllPingTasks", rpc.RoleAdmin, ping.AdminListTasks, &rpc.MethodMeta{
		Name:    "admin:getAllPingTasks",
		Summary: "List all ping tasks",
		Returns: "PingTask[]",
	})
	RegisterWithGroupAndMeta("orderPingTask", rpc.RoleAdmin, ping.AdminOrderTasks, &rpc.MethodMeta{
		Name:    "admin:orderPingTask",
		Summary: "Reorder ping tasks (map of id->weight)",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("applyBuiltinPingPresets", rpc.RoleAdmin, ping.AdminApplyBuiltinPingPresets, &rpc.MethodMeta{
		Name:    "admin:applyBuiltinPingPresets",
		Summary: "Apply selected built-in China 31-province/carrier ping nodes to servers",
		Returns: "{ created: int, updated: int }",
	})
}
