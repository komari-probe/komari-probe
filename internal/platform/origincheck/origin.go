package origincheck

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/pkg/kv"
)

func SplitAllowlist(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func OriginMatchesHost(origin, host string) bool {
	_, originHost, ok := normalizeOrigin(origin)
	return ok && strings.EqualFold(originHost, host)
}

func OriginInAllowlist(origin, rawAllowlist string) bool {
	normalizedOrigin, originHost, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	for _, entry := range SplitAllowlist(rawAllowlist) {
		if entry == "*" {
			return true
		}
		if strings.Contains(entry, "://") {
			normalizedEntry, _, ok := normalizeOrigin(entry)
			if ok && strings.EqualFold(normalizedEntry, normalizedOrigin) {
				return true
			}
			continue
		}
		if strings.EqualFold(entry, originHost) {
			return true
		}
	}
	return false
}

// CheckWebSocketOrigin reports whether a WebSocket upgrade request's Origin
// is acceptable: API-key requests and token-authenticated requests without
// an Origin header bypass the check, KOMARI_WS_DISABLE_ORIGIN=true disables
// it entirely, and otherwise the origin must match the request host or be
// listed in the configured allowlist.
func CheckWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if strings.EqualFold(os.Getenv("KOMARI_WS_DISABLE_ORIGIN"), "true") {
		return true
	}
	if IsAPIKeyRequest(r) {
		return true
	}
	if origin == "" && r.URL.Query().Get("token") != "" {
		return true
	}
	enabled, _ := kv.GetAs[bool](settings.WsOriginCheckEnabledKey, true)
	if !enabled {
		return true
	}
	if origin == "" {
		return false
	}
	if OriginMatchesHost(origin, r.Host) {
		return true
	}
	allowlist, _ := kv.GetAs[string](settings.WsAllowedOriginsKey, "")
	return OriginInAllowlist(origin, allowlist)
}

func IsAPIKeyRequest(r *http.Request) bool {
	apiKeyConfig, err := kv.GetAs[string](settings.APIKeyKey, "")
	if err != nil || apiKeyConfig == "" || len(apiKeyConfig) < 12 {
		return false
	}
	provided := r.Header.Get("Authorization")
	expected := "Bearer " + apiKeyConfig
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func IsAuthorizationPreflight(r *http.Request) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		if strings.EqualFold(strings.TrimSpace(header), "authorization") {
			return true
		}
	}
	return false
}

func normalizeOrigin(raw string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", false
	}
	host := strings.ToLower(parsed.Host)
	return strings.ToLower(parsed.Scheme) + "://" + host, host, true
}
