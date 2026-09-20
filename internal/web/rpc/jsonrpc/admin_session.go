package jsonrpc

import (
	"github.com/komari-monitor/komari/internal/features/auth"
	"github.com/komari-monitor/komari/pkg/rpc"
)

// admin_session.go
// 会话管理的 RPC2 方法注册（admin 命名空间）。处理逻辑在 internal/features/auth，
// 这里只做注册。

func init() {
	RegisterWithGroupAndMeta("getSessions", rpc.RoleAdmin, auth.AdminGetSessions, &rpc.MethodMeta{
		Name:    "admin:getSessions",
		Summary: "List all login sessions",
		Returns: "{ current: string, data: Session[] }",
	})
	RegisterWithGroupAndMeta("deleteSession", rpc.RoleAdmin, auth.AdminDeleteSession, &rpc.MethodMeta{
		Name:    "admin:deleteSession",
		Summary: "Delete a session by token",
		Returns: "null",
	})
	RegisterWithGroupAndMeta("deleteAllSessions", rpc.RoleAdmin, auth.AdminDeleteAllSessions, &rpc.MethodMeta{
		Name:    "admin:deleteAllSessions",
		Summary: "Delete all sessions",
		Returns: "null",
	})
}
