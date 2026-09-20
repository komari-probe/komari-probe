package jsonrpc

import (
	"context"

	"github.com/komari-monitor/komari/pkg/rpc"
)

// auditActor 从上下文提取审计用的 actor UUID 与来源 IP。
func auditActor(ctx context.Context) (uuid, ip string) {
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		uuid = meta.UserUUID
		ip = meta.RemoteIP
	}
	return uuid, ip
}

// Register 以默认分组 "common" 注册方法。
func Register(name string, cb rpc.Handler) error {
	return RegisterWithGroupAndMeta(name, "common", cb, &rpc.MethodMeta{
		Name:        name,
		Summary:     "This method does not provide a summary",
		Description: "This method does not provide a description",
	})
}

// reg 是 admin 命名空间方法的注册便捷封装。
func reg(name string, h rpc.Handler, summary string) {
	RegisterWithGroupAndMeta(name, rpc.RoleAdmin, h, &rpc.MethodMeta{Name: "admin:" + name, Summary: summary})
}

// RegisterWithGroupAndMeta 将回调按分组注册为 "group:name"，并附加元数据。
// group 为空时使用默认分组 "common"。
func RegisterWithGroupAndMeta(name, group string, cb rpc.Handler, meta *rpc.MethodMeta) error {
	if group == "" {
		group = "common"
	}
	method := group + ":" + name
	if meta == nil {
		meta = &rpc.MethodMeta{}
	}
	if meta.Name == "" || meta.Name == name {
		meta.Name = method
	}
	return rpc.RegisterWithMeta(method, cb, meta)
}
