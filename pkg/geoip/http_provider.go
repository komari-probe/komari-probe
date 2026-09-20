package geoip

import "time"

// httpProviderTimeout 是所有轻量级 HTTP provider 共用的请求超时时间。
const httpProviderTimeout = 5 * time.Second

// noopLifecycle 为无本地状态、依赖外部 Web 服务的 provider 提供空操作的
// UpdateDatabase/Close 实现：数据由远端服务持有，本地既无需下载也无需释放资源。
type noopLifecycle struct{}

func (noopLifecycle) UpdateDatabase() error { return nil }
func (noopLifecycle) Close() error          { return nil }
