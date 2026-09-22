package respond

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sonar-probe/sonar/internal/platform/origincheck"
	"github.com/sonar-probe/sonar/pkg/wsconn"
)

// WebSocketUpgradeOption customizes the upgrader used by UpgradeWebSocket.
type WebSocketUpgradeOption func(*websocket.Upgrader)

// IsWebSocketUpgrade reports whether the request is a WebSocket upgrade request.
func IsWebSocketUpgrade(c *gin.Context) bool {
	return websocket.IsWebSocketUpgrade(c.Request)
}

// EnableWebSocketCompression turns on per-message compression on the upgrader.
func EnableWebSocketCompression(upgrader *websocket.Upgrader) {
	upgrader.EnableCompression = true
}

// UpgradeWebSocket upgrades an incoming HTTP request to a WebSocket connection.
func UpgradeWebSocket(c *gin.Context, options ...WebSocketUpgradeOption) (*websocket.Conn, error) {
	if !IsWebSocketUpgrade(c) {
		return nil, fmt.Errorf("require websocket upgrade")
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: origincheck.CheckWebSocketOrigin,
	}
	for _, option := range options {
		option(&upgrader)
	}
	return upgrader.Upgrade(c.Writer, c.Request, nil)
}

// UpgradeSafeConn upgrades the request to a WebSocket and attaches the
// process-wide plugin frame interceptor (when one is wired). A plugin
// wsConnect hook may deny the connection: the peer receives a
// policy-violation close frame and the caller gets an error.
func UpgradeSafeConn(c *gin.Context, options ...WebSocketUpgradeOption) (*wsconn.SafeConn, error) {
	unsafeConn, err := UpgradeWebSocket(c, options...)
	if err != nil {
		return nil, err
	}
	interceptor := wsconn.Interceptor()
	if interceptor == nil {
		return wsconn.NewSafeConn(unsafeConn), nil
	}
	sc := wsconn.NewSafeConn(unsafeConn)
	info := &wsconn.ConnInfo{
		ID:        sc.ID,
		Path:      c.Request.URL.Path,
		RemoteIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if clientUUID, ok := c.Get("client_uuid"); ok {
		if s, ok := clientUUID.(string); ok {
			info.ClientUUID = s
		}
	}
	if deny, reason := interceptor.OnConnect(info); deny {
		_ = unsafeConn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason),
			time.Now().Add(time.Second))
		_ = unsafeConn.Close()
		return nil, errors.New("websocket connection denied by plugin")
	}
	sc.SetInterceptor(info, interceptor)
	return sc, nil
}
