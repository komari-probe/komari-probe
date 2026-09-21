package auth

import (
	"context"

	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// handler_admin_session.go
// 会话管理的 RPC2 方法处理逻辑（admin 命名空间）。方法注册留在 transport/jsonrpc。

func AdminGetSessions(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	ss, err := GetAllSessions()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to retrieve sessions: "+err.Error(), nil)
	}
	current := ""
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		current = meta.SessionToken
	}
	return map[string]any{"current": current, "data": ss}, nil
}

func AdminDeleteSession(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Session string `json:"session"`
	}
	req.BindParams(&params)
	if params.Session == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "session is required", nil)
	}
	if err := DeleteSession(params.Session); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete session: "+err.Error(), nil)
	}
	actor, ip := rpc.ActorFromContext(ctx)
	auditlog.Log(ip, actor, "delete session", "info")
	return nil, nil
}

func AdminDeleteAllSessions(ctx context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	if err := DeleteAllSessions(); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to delete all sessions: "+err.Error(), nil)
	}
	actor, ip := rpc.ActorFromContext(ctx)
	auditlog.Log(ip, actor, "delete all sessions", "warn")
	return nil, nil
}
