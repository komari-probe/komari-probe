package node

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/komari-monitor/komari/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/komari-monitor/komari/internal/platform/clients"
	"github.com/komari-monitor/komari/internal/platform/protocol"
	"github.com/komari-monitor/komari/internal/platform/respond"
	"github.com/komari-monitor/komari/pkg/wsconn"
)

func readMaybeCompressedBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	}
	return io.ReadAll(r.Body)
}

func bindV2Params[T any](raw any, target *T) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func handleV2RPC(uuid string, req protocol.Request, allowWait bool) protocol.Response {
	if req.JSONRPC != protocol.Version {
		return protocol.Error(req.ID, -32600, "invalid jsonrpc version", nil)
	}
	switch req.Method {
	case protocol.MethodAgentReport:
		var params protocol.ReportParams
		if err := bindV2Params(req.Params, &params); err != nil {
			return protocol.Error(req.ID, -32602, "invalid report params", err.Error())
		}
		if err := ingestReport(uuid, params.Report, true); err != nil {
			return protocol.Error(req.ID, -32000, "failed to save report", err.Error())
		}
		return protocol.Success(req.ID, gin.H{
			"status": "success",
			"events": TakeV2Events(uuid, params.AckEventIDs, 8),
		})
	case protocol.MethodAgentBasicInfo:
		var params protocol.BasicInfoParams
		if err := bindV2Params(req.Params, &params); err != nil {
			return protocol.Error(req.ID, -32602, "invalid basic info params", err.Error())
		}
		if err := ingestBasicInfo(uuid, params.Info, ""); err != nil {
			return protocol.Error(req.ID, -32000, "failed to save basic info", err.Error())
		}
		return protocol.Success(req.ID, gin.H{"status": "success"})
	case protocol.MethodAgentPingResult:
		var params protocol.PingResultParams
		if err := bindV2Params(req.Params, &params); err != nil {
			return protocol.Error(req.ID, -32602, "invalid ping result params", err.Error())
		}
		if err := ingestPingResult(uuid, params.TaskID, params.Value); err != nil {
			return protocol.Error(req.ID, -32000, "failed to save ping result", err.Error())
		}
		return protocol.Success(req.ID, gin.H{"status": "success"})
	case protocol.MethodAgentPull:
		var params protocol.PullParams
		if err := bindV2Params(req.Params, &params); err != nil {
			return protocol.Error(req.ID, -32602, "invalid pull params", err.Error())
		}
		refreshPostPresence(uuid)
		MarkV2Client(uuid)
		timeout := 0 * time.Second
		if allowWait {
			timeout = 25 * time.Second
		}
		return protocol.Success(req.ID, gin.H{
			"events": WaitV2Events(uuid, params.AckEventIDs, timeout),
		})
	default:
		return protocol.Error(req.ID, -32601, "method not found", req.Method)
	}
}

func UploadV2RPC(c *gin.Context) {
	bytesBody, err := readMaybeCompressedBody(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, protocol.Error(nil, -32700, "invalid compressed body", err.Error()))
		return
	}
	var req protocol.Request
	if err := json.Unmarshal(bytesBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, protocol.Error(nil, -32700, "parse error", err.Error()))
		return
	}
	uuid, ok := clientUUIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, protocol.Error(req.ID, -32001, "invalid token", nil))
		return
	}
	resp := handleV2RPC(uuid, req, true)
	status := http.StatusOK
	if resp.Error != nil {
		status = http.StatusBadRequest
	}
	c.JSON(status, resp)
}

func WebSocketV2RPC(c *gin.Context) {
	if !respond.IsWebSocketUpgrade(c) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Require WebSocket upgrade"})
		return
	}
	conn, err := respond.UpgradeSafeConn(c, respond.EnableWebSocketCompression)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Failed to upgrade to WebSocket." + err.Error()})
		return
	}
	defer conn.Close()

	uuid, ok := clientUUIDFromContext(c)
	if !ok {
		conn.WriteJSON(protocol.Error(nil, -32001, "invalid token", nil))
		return
	}
	if oldConn, exists := GetConnectedClients()[uuid]; exists {
		go oldConn.Close()
	}
	SetConnectedClients(uuid, conn)
	MarkV2Client(uuid)
	go notifyOnline(uuid, conn.ID)
	defer func() {
		DeleteClientConditionally(uuid, conn)
		notifyOffline(uuid, conn.ID)
	}()
	if !pushQueuedV2Events(conn, uuid) {
		return
	}

	for {
		conn.SetReadDeadline(time.Now().Add(readWait))
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Errorf("client-api", "Client %s v2 connection error: %v", uuid, err)
			}
			return
		}
		message = bytes.TrimSpace(message)
		var req protocol.Request
		if err := json.Unmarshal(message, &req); err != nil {
			conn.WriteJSON(protocol.Error(nil, -32700, "parse error", err.Error()))
			continue
		}
		resp := handleV2RPC(uuid, req, false)
		if req.ID != nil {
			if err := conn.WriteJSON(resp); err != nil {
				logger.Errorf("client-api", "failed to write v2 rpc response: %v", err)
				return
			}
		}
	}
}

func pushQueuedV2Events(conn *wsconn.SafeConn, uuid string) bool {
	events := TakeV2Events(uuid, nil, 0)
	if len(events) == 0 {
		return true
	}
	ackIDs := make([]string, 0, len(events))
	for _, event := range events {
		payload := protocol.Request{JSONRPC: protocol.Version, Method: event.Method, Params: event.Params}
		if err := conn.WriteJSON(payload); err != nil {
			AckV2Events(uuid, ackIDs)
			logger.Errorf("client-api", "failed to push queued v2 event %s to client %s: %v", event.ID, uuid, err)
			return false
		}
		ackIDs = append(ackIDs, event.ID)
	}
	AckV2Events(uuid, ackIDs)
	return true
}

func clientUUIDFromContext(c *gin.Context) (string, bool) {
	if v, ok := c.Get("client_uuid"); ok {
		if uuid, ok := v.(string); ok && uuid != "" {
			return uuid, true
		}
	}
	token := c.Query("token")
	if token == "" {
		return "", false
	}
	uuid, err := clients.GetClientUUIDByToken(token)
	return uuid, err == nil && uuid != ""
}
