package client

import (
	"context"

	agent_runtime "github.com/komari-monitor/komari/internal/platform/agent"
	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/internal/platform/clients"
	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/metricstore"
	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/internal/platform/records"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// handler_admin.go
// client 资源的 RPC2 方法处理逻辑（admin 命名空间）。方法注册留在 web/rpc/jsonrpc。

// auditActor 从上下文提取审计用的 actor UUID 与来源 IP。
func auditActor(ctx context.Context) (uuid, ip string) {
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		uuid = meta.UserUUID
		ip = meta.RemoteIP
	}
	return uuid, ip
}

func AdminAddClient(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Name string `json:"name"`
	}
	req.BindParams(&params)

	uuid, token, err := createClientWithDefaults(params.Name)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	if params.Name != "" {
		actor, ip := auditActor(ctx)
		auditlog.Log(ip, actor, "create client:"+uuid, "info")
	}
	return map[string]any{"uuid": uuid, "token": token}, nil
}

func AdminEditClient(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var update map[string]interface{}
	if err := req.BindParams(&update); err != nil || update == nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid params", nil)
	}
	uuid, _ := update["uuid"].(string)
	if uuid == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	if err := clients.SaveClient(update); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "edit client:"+uuid, "info")
	return nil, nil
}

func AdminRemoveClient(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	if err := clients.DeleteClient(params.UUID); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete client"+err.Error(), nil)
	}
	metricstore.DeleteEntityAsync(params.UUID)
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "delete client:"+params.UUID, "warn")
	agent_runtime.DeleteConnectedClients(params.UUID)
	agent_runtime.DeleteLatestReport(params.UUID)
	return nil, nil
}

func AdminGetClient(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	result, err := clients.GetClientByUUID(params.UUID)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return result, nil
}

func AdminListClients(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	cls, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return cls, nil
}

func AdminGetClientToken(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		UUID string `json:"uuid"`
	}
	req.BindParams(&params)
	if params.UUID == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing UUID", nil)
	}
	token, err := clients.GetClientTokenByUUID(params.UUID)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"token": token}, nil
}

func AdminClearRecords(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if err := records.DeleteAll(); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete Record"+err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "clear records", "warn")
	return nil, nil
}

func AdminOrderClients(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var order map[string]int
	if err := req.BindParams(&order); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid or missing request body: "+err.Error(), nil)
	}
	db := dbcore.GetDBInstance()
	for uuid, weight := range order {
		if err := db.Model(&models.Client{}).Where("uuid = ?", uuid).Update("weight", weight).Error; err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to update client weight: "+err.Error(), nil)
		}
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "order clients", "info")
	return nil, nil
}
