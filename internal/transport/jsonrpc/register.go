package jsonrpc

import (
	"context"
	"fmt"

	"github.com/sonar-probe/sonar/pkg/rpc"
)

// auditActor 从上下文提取审计用的 actor UUID 与来源 IP。
func auditActor(ctx context.Context) (uuid, ip string) {
	if meta := rpc.MetaFromContext(ctx); meta != nil {
		uuid = meta.UserUUID
		ip = meta.RemoteIP
	}
	return uuid, ip
}

// Register 以默认分组 "common" 注册方法。只应在 init() 里调用：注册失败会
// panic（见 RegisterWithGroupAndMeta），没有 error 可返回给调用方检查。
func Register(name string, cb rpc.Handler) {
	RegisterWithGroupAndMeta(name, "common", cb, &rpc.MethodMeta{
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
//
// 只应在 init() 里调用，因此不返回 error：注册失败（最常见的原因是方法名
// 与别处撞了）说明代码有 bug，必须让启动直接 panic 并报出冲突的方法名，
// 而不是悄悄把这个方法从路由表里消失——包里原来没有一处检查过这里的错误，
// 与其让调用方继续忽略一个"看似被处理、实则没人看"的返回值，不如干脆
// 不给它返回值。
func RegisterWithGroupAndMeta(name, group string, cb rpc.Handler, meta *rpc.MethodMeta) {
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
	if err := rpc.RegisterWithMeta(method, cb, meta); err != nil {
		panic(fmt.Sprintf("jsonrpc: register %q: %v", method, err))
	}
}
