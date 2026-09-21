package auth

import (
	"context"

	"github.com/komari-monitor/komari/internal/features/auth/oauth"
	oauthfactory "github.com/komari-monitor/komari/internal/features/auth/oauth/factory"
	"github.com/komari-monitor/komari/internal/platform/models"
	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// handler_admin_provider.go
// OIDC provider 配置的 RPC2 方法处理逻辑（admin 命名空间）。方法注册留在
// transport/jsonrpc/admin_provider.go。

func AdminGetOidc(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Provider string `json:"provider"`
	}
	req.BindParams(&params)
	if params.Provider != "" {
		cfg, err := oauth.GetOidcConfigByName(params.Provider)
		if err != nil {
			return nil, rpc.MakeError(rpc.NotFound, "Provider not found: "+err.Error(), nil)
		}
		return cfg, nil
	}
	providers := oauthfactory.GetProviderConfigs()
	if len(providers) == 0 {
		return nil, rpc.MakeError(rpc.NotFound, "No OIDC providers found", nil)
	}
	return providers, nil
}

func AdminSetOidc(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var oidcConfig models.OidcProvider
	if err := req.BindParams(&oidcConfig); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid configuration: "+err.Error(), nil)
	}
	if oidcConfig.Name == "" {
		return nil, rpc.MakeError(rpc.InvalidParams, "Provider name is required", nil)
	}
	if _, exists := oauthfactory.GetConstructor(oidcConfig.Name); !exists {
		return nil, rpc.MakeError(rpc.NotFound, "Provider not found: "+oidcConfig.Name, nil)
	}
	if err := oauth.SaveOidcConfig(&oidcConfig); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to save OIDC provider configuration: "+err.Error(), nil)
	}
	provider, _ := kv.GetAs[string](settings.OAuthProviderKey, "github")
	if provider == oidcConfig.Name { // 正在使用，重载
		if err := oauth.LoadProvider(oidcConfig.Name, oidcConfig.Addition); err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to load OIDC provider: "+err.Error(), nil)
		}
	}
	return map[string]any{"message": "OIDC provider set successfully"}, nil
}
