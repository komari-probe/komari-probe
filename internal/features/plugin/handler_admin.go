package plugin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// handler_admin.go
// 插件相关 admin 接口：静态文件服务（REST）+ RPC2 方法处理逻辑。
// RPC2 方法注册留在 web/jsonrpc/admin_plugin.go。

// ServeAdminPluginFile serves a static file from an installed plugin directory,
// used by injected plugin admin pages. 与 ServePublicPluginFile（无需鉴权的公开
// iframe 页面）同名会冲突，故以 Admin/Public 区分。
func ServeAdminPluginFile(c *gin.Context) {
	name := strings.TrimPrefix(c.Param("filepath"), "/")
	full, err := ResolveFile(c.Param("short"), name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(full)
}

func AdminListPlugins(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return List(), nil
}

// AdminSetPluginEnabled is the single switch entry point. When a plugin's
// declared permissions differ from the approved hash, enabling it returns
// { requires_approval: true } instead of an error; the caller then shows a
// permission dialog and retries with approved=true.
func AdminSetPluginEnabled(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	enabled, _ := rpc.GetParamAs[bool](req, "enabled")
	approved, _ := rpc.GetParamAs[bool](req, "approved")
	if err := SetEnabled(short, enabled, approved); err != nil {
		if errors.Is(err, ErrPermissionApprovalRequired) {
			return map[string]any{"requires_approval": true}, nil
		}
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func AdminGetPluginLogs(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	return map[string]any{"logs": GetLogs(short)}, nil
}

func AdminDeletePlugin(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	if err := Delete(short); err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return nil, rpc.MakeError(rpc.NotFound, err.Error(), nil)
		}
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return nil, nil
}

func AdminGetPluginConfiguration(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	info, err := Manifest(short)
	if err != nil {
		return nil, rpc.MakeError(rpc.NotFound, err.Error(), nil)
	}
	values, err := GetConfiguration(short)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return map[string]any{"configuration": info.Configuration, "data": values}, nil
}

func AdminSetPluginConfiguration(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	short, ok := rpc.GetParamAs[string](req, "short")
	if !ok || strings.TrimSpace(short) == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "short is required", nil)
	}
	data, _ := rpc.GetParamAs[map[string]any](req, "data")
	if err := SaveConfiguration(short, data); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	if err := Reload(short); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "plugin configuration saved but reload failed: "+err.Error(), nil)
	}
	return nil, nil
}
