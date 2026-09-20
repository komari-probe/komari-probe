package rpc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Handler 方法签名：返回 result (成功) 或 *JsonRpcError (失败)
type Handler func(ctx context.Context, req *JsonRpcRequest) (any, *JsonRpcError)

var (
	muHandlers sync.RWMutex
	handlers   = map[string]Handler{}
)

// Register 注册方法。重复注册返回错误。保留前缀 "rpc." 禁止外部注册。
func Register(method string, h Handler) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return errors.New("method empty")
	}
	if strings.HasPrefix(method, "rpc.") {
		return errors.New("method prefix 'rpc.' is reserved")
	}
	muHandlers.Lock()
	if _, exists := handlers[method]; exists {
		muHandlers.Unlock()
		return fmt.Errorf("method already registered: %s", method)
	}
	handlers[method] = h
	muHandlers.Unlock()
	// Every registered method must be inspectable through rpc.help, including
	// plugin-owned methods that do not provide explicit metadata.
	ensureMeta(method)
	return nil
}

// MustRegister 便捷注册（panic on error）
func MustRegister(method string, h Handler) {
	if err := Register(method, h); err != nil {
		panic(err)
	}
}

// Unregister 注销已注册的方法，并清理其元数据。
// 保留前缀 "rpc." 的内部方法禁止注销。返回是否存在并被移除。
// 主要供插件卸载时动态移除其注册的方法。
func Unregister(method string) bool {
	method = strings.TrimSpace(method)
	if method == "" || strings.HasPrefix(method, "rpc.") {
		return false
	}
	muHandlers.Lock()
	_, exists := handlers[method]
	if exists {
		delete(handlers, method)
	}
	muHandlers.Unlock()
	if exists {
		muMetas.Lock()
		delete(methodMetas, method)
		muMetas.Unlock()
	}
	return exists
}

// ListMethods 列出当前已注册的方法名（副本）
func ListMethods() []string {
	muHandlers.RLock()
	defer muHandlers.RUnlock()
	res := make([]string, 0, len(handlers))
	for k := range handlers {
		res = append(res, k)
	}
	return res
}

// listMethods 返回方法列表；includeInternal=false 时剔除 rpc.*
func listMethods(includeInternal bool) []string {
	all := ListMethods()
	out := make([]string, 0, len(all))
	for _, m := range all {
		if !includeInternal && strings.HasPrefix(m, "rpc.") {
			continue
		}
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// lookupHandler returns the registered handler for method, or a MethodNotFound
// error.
func lookupHandler(method string) (Handler, *JsonRpcError) {
	muHandlers.RLock()
	h, ok := handlers[method]
	muHandlers.RUnlock()
	if !ok {
		return nil, &JsonRpcError{Code: MethodNotFound, Message: "method not found", Data: method}
	}
	return h, nil
}
