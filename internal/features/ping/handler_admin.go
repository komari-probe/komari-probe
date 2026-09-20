package ping

import (
	"context"
	"strconv"

	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// handler_admin.go
// 延迟监测任务（ping task）RPC2 方法的处理逻辑（admin 命名空间）。
// 方法注册（RegisterWithGroupAndMeta）留在 web/rpc/jsonrpc 里，
// 因为注册中心本身在那边；这里只暴露纯逻辑函数，避免 ping 反过来
// 依赖 jsonrpc 造成循环 import。

func AdminAddTask(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Clients   []string `json:"clients"`
		DefaultOn bool     `json:"default_on"`
		Name      string   `json:"name"`
		Target    string   `json:"target"`
		TaskType  string   `json:"type"`
		Interval  int      `json:"interval"`
	}
	req.BindParams(&params)
	if params.Name == "" || params.Target == "" || params.TaskType == "" || params.Interval == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "name, target, type and interval are required", nil)
	}
	if !params.DefaultOn && len(params.Clients) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "clients is required when default_on is false", nil)
	}
	taskID, err := AddPingTask(params.Clients, params.DefaultOn, params.Name, params.Target, params.TaskType, params.Interval)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"task_id": taskID}, nil
}

func AdminDeleteTask(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		ID []uint `json:"id"`
	}
	req.BindParams(&params)
	if len(params.ID) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "id is required", nil)
	}
	if err := DeletePingTask(params.ID); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func AdminEditTask(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Tasks []*models.PingTask `json:"tasks"`
	}
	req.BindParams(&params)
	if len(params.Tasks) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data", nil)
	}
	for _, task := range params.Tasks {
		if task == nil {
			return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data", nil)
		}
	}
	if err := EditPingTask(params.Tasks); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func AdminListTasks(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	list, err := GetAllPingTasks()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return list, nil
}

func AdminOrderTasks(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	// 参数为 { idStr: weight } 映射。
	order := map[uint]int{}
	var raw map[string]int
	if err := req.BindParams(&raw); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing request body: "+err.Error(), nil)
	}
	for idStr, weight := range raw {
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			return nil, rpc.MakeError(rpc.InvalidParams, "Invalid task id: "+idStr, nil)
		}
		order[uint(id)] = weight
	}
	if err := UpdatePingTaskOrder(order); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}
