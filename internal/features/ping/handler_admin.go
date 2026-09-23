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

// AdminApplyBuiltinPingPresets 把管理后台"内置监测节点"选择器里勾选的
// 省份/运营商/IP版本组合应用到选中的服务器上。省份/运营商代码由服务端
// 用 pingpresets.Find 校验，不直接信任前端拼出来的 target 字符串。
func AdminApplyBuiltinPingPresets(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Nodes []struct {
			ProvinceCode string `json:"province_code"`
			CarrierCode  string `json:"carrier_code"`
			IPVersion    int    `json:"ip_version"`
		} `json:"nodes"`
		Clients []string `json:"clients"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request data: "+err.Error(), nil)
	}
	if len(params.Nodes) == 0 || len(params.Clients) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "nodes and clients are required", nil)
	}

	resolved := make([]pingpresets.Node, 0, len(params.Nodes))
	for _, n := range params.Nodes {
		node, ok := pingpresets.Find(n.ProvinceCode, n.CarrierCode, n.IPVersion)
		if !ok {
			return nil, rpc.MakeError(rpc.InvalidParams, fmt.Sprintf("unknown builtin node: %s-%s-v%d", n.ProvinceCode, n.CarrierCode, n.IPVersion), nil)
		}
		resolved = append(resolved, node)
	}

	created, updated, err := ApplyBuiltinPingPresets(resolved, params.Clients)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"created": created, "updated": updated}, nil
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
