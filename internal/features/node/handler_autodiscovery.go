package node

import (
	"crypto/subtle"

	"github.com/gin-gonic/gin"
	"github.com/sonar-probe/sonar/internal/platform/respond"
	"github.com/sonar-probe/sonar/internal/platform/settings"
	"github.com/sonar-probe/sonar/pkg/kv"
	"github.com/sonar-probe/sonar/pkg/random"
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
	expected := "Bearer " + AutoDiscoveryKey
	if AutoDiscoveryKey == "" ||
		len(AutoDiscoveryKey) < 12 ||
		subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {

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
