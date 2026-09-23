package ping

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/pingpresets"
	"github.com/sonar-probe/sonar/pkg/rpc"
)

// handler_admin.go
// 延迟监测任务（ping task）RPC2 方法的处理逻辑（admin 命名空间）。
// 方法注册（RegisterWithGroupAndMeta）留在 transport/jsonrpc 里，
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
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data: "+err.Error(), nil)
	}
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
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data: "+err.Error(), nil)
	}
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
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data: "+err.Error(), nil)
	}
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

// AdminSyncClientPingNodes 全量同步"这台服务器要监测哪些节点"，供服务器
// 列表页面的"设置监测节点"弹窗调用：内置节点按省份/运营商代码校验（不信任
// 前端拼出来的 target 字符串），自建任务按 ID 传入。未出现在这次提交里的
// 节点，会把这台服务器从对应任务里移除。
func AdminSyncClientPingNodes(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Client   string `json:"client"`
		Interval int    `json:"interval"`
		Builtin  []struct {
			ProvinceCode string `json:"province_code"`
			CarrierCode  string `json:"carrier_code"`
		} `json:"builtin"`
		CustomTaskIDs []uint `json:"custom_task_ids"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data: "+err.Error(), nil)
	}
	if params.Client == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "client is required", nil)
	}

	resolved := make([]pingpresets.Node, 0, len(params.Builtin))
	for _, n := range params.Builtin {
		node, ok := pingpresets.Find(n.ProvinceCode, n.CarrierCode, 4)
		if !ok {
			return nil, rpc.MakeError(rpc.InvalidParams, fmt.Sprintf("unknown builtin node: %s-%s", n.ProvinceCode, n.CarrierCode), nil)
		}
		resolved = append(resolved, node)
	}

	if err := SyncClientPingNodes(params.Client, resolved, params.Interval, params.CustomTaskIDs); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
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
