package jsonrpc

import (
	"context"
	"errors"

	"github.com/komari-monitor/komari/internal/platform/auditlog"
	"github.com/komari-monitor/komari/pkg/rpc"
)

type visitorAuditParams struct {
	Event     string         `json:"event"`
	Action    string         `json:"action"`
	Operation string         `json:"operation"`
	Path      string         `json:"path"`
	Route     string         `json:"route"`
	Target    string         `json:"target"`
	Detail    map[string]any `json:"detail"`
}

func init() {
	RegisterWithGroupAndMeta("recordVisitorEvent", "public", publicRecordVisitorEvent, &rpc.MethodMeta{
		Name:    "public:recordVisitorEvent",
		Summary: "Record a frontend visitor audit event",
		Params: []rpc.ParamMeta{
			{Name: "event", Type: "string", Required: true, Description: "Short event name, such as page_view or node_open"},
			{Name: "action", Type: "string", Description: "Alias of event"},
			{Name: "operation", Type: "string", Description: "Alias of event"},
			{Name: "path", Type: "string", Description: "Frontend path or URL path, without secrets"},
			{Name: "route", Type: "string", Description: "Frontend route name"},
			{Name: "target", Type: "string", Description: "Optional target identifier"},
			{Name: "detail", Type: "object", Description: "Optional bounded metadata supplied by the frontend"},
		},
		Returns: "{ status: string }",
	})
}

func publicRecordVisitorEvent(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	meta := rpc.MetaFromContext(ctx)
	ip := ""
	uuid := ""
	userAgent := ""
	if meta != nil {
		ip = meta.RemoteIP
		uuid = meta.UserUUID
		userAgent = meta.UserAgent
	}

	if !auditlog.AllowVisitorEvent(ip) {
		return map[string]any{"status": "rate_limited"}, nil
	}
	enabled, err := auditlog.VisitorAuditEnabled()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, "Failed to get visitor audit configuration", nil)
	}
	if !enabled {
		return map[string]any{"status": "disabled"}, nil
	}

	var params visitorAuditParams
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid params: "+err.Error(), nil)
	}

	err = auditlog.RecordVisitorEvent(auditlog.VisitorEvent{
		IP:        ip,
		UUID:      uuid,
		UserAgent: userAgent,
		Event:     firstNonEmpty(params.Event, params.Action, params.Operation),
		Path:      params.Path,
		Route:     params.Route,
		Target:    params.Target,
		Detail:    params.Detail,
	})
	if err != nil {
		if errors.Is(err, auditlog.ErrVisitorEventRequired) {
			return nil, rpc.MakeError(rpc.InvalidParams, "event is required", nil)
		}
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid detail", nil)
	}
	return map[string]any{"status": "success"}, nil
}
