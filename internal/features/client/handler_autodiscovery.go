package client

import (
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/platform/respond"
	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/pkg/kv"
	"github.com/komari-monitor/komari/pkg/random"
)

func RegisterClient(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		respond.Error(c, 403, "Invalid AutoDiscovery Key")
		return
	}
	AutoDiscoveryKey, err := kv.GetAs[string](settings.AutoDiscoveryKeyKey, "")
	if err != nil {
		respond.Error(c, 500, "Failed to get AutoDiscovery Key: "+err.Error())
		return
	}
	if AutoDiscoveryKey == "" ||
		len(AutoDiscoveryKey) < 12 ||
		"Bearer "+AutoDiscoveryKey != auth {

		respond.Error(c, 403, "Invalid AutoDiscovery Key")
		return
	}
	name := c.Query("name")
	if name == "" {
		name = random.String(8)
	}
	name = "Auto-" + name
	uuid, token, err := createClientWithDefaults(name)
	if err != nil {
		respond.Error(c, 500, "Failed to create client: "+err.Error())
		return
	}
	respond.Success(c, gin.H{"uuid": uuid, "token": token})
}
