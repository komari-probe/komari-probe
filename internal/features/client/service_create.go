package client

import (
	logger "github.com/komari-monitor/komari/pkg/log"

	"github.com/komari-monitor/komari/internal/features/ping"
	"github.com/komari-monitor/komari/internal/platform/clients"
)

// service_create.go
// 创建客户端的编排逻辑：落库之后为新客户端应用默认开启的 ping 任务。
// platform/clients 只负责纯粹的数据存取，不感知 ping 这个 feature。

func createClientWithDefaults(name string) (uuid, token string, err error) {
	if name == "" {
		uuid, token, err = clients.CreateClient()
	} else {
		uuid, token, err = clients.CreateClientWithName(name)
	}
	if err != nil {
		return "", "", err
	}
	if err := ping.AddDefaultOnClientUUID(uuid); err != nil {
		logger.ErrorArgs("client", "Failed to apply default-on ping tasks to new client:", err)
	}
	return uuid, token, nil
}
