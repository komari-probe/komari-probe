package jsonrpc

import (
	"github.com/komari-monitor/komari/internal/features/plugin"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// admin_plugin.go
// 插件相关 RPC2 方法注册（admin 命名空间）。处理逻辑在 internal/features/plugin，
// 这里只做注册。

func init() {
	RegisterWithGroupAndMeta("listPlugins", rpc.RoleAdmin, plugin.AdminListPlugins, &rpc.MethodMeta{
		Name:    "admin:listPlugins",
		Summary: "List installed plugins with enabled/running state",
		Returns: "Plugin[]",
	})
	RegisterWithGroupAndMeta("setPluginEnabled", rpc.RoleAdmin, plugin.AdminSetPluginEnabled, &rpc.MethodMeta{
		Name:    "admin:setPluginEnabled",
		Summary: "Enable or disable a plugin by short name",
		Returns: "null | { requires_approval: true }",
	})
	RegisterWithGroupAndMeta("getPluginLogs", rpc.RoleAdmin, plugin.AdminGetPluginLogs, &rpc.MethodMeta{
		Name:    "admin:getPluginLogs",
		Summary: "Get the bounded runtime log buffer of a plugin",
		Returns: "{ logs: string }",
	})
	RegisterWithGroupAndMeta("deletePlugin", rpc.RoleAdmin, plugin.AdminDeletePlugin, &rpc.MethodMeta{
		Name:    "admin:deletePlugin",
		Summary: "Delete an installed plugin and its persisted state",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("getPluginConfiguration", rpc.RoleAdmin, plugin.AdminGetPluginConfiguration, &rpc.MethodMeta{
		Name:    "admin:getPluginConfiguration",
		Summary: "Get a plugin's declared config items and saved values",
		Returns: "{ configuration: object, data: object }",
	})
	RegisterWithGroupAndMeta("setPluginConfiguration", rpc.RoleAdmin, plugin.AdminSetPluginConfiguration, &rpc.MethodMeta{
		Name:    "admin:setPluginConfiguration",
		Summary: "Save a plugin's configuration values",
		Returns: "null",
	})
}
