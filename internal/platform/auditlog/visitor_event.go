package auditlog

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/komari-monitor/komari/internal/platform/settings"
	"github.com/komari-monitor/komari/pkg/kv"
)

const (
	visitorAuditMaxEventLen     = 64
	visitorAuditMaxPathLen      = 512
	visitorAuditMaxRouteLen     = 128
	visitorAuditMaxTargetLen    = 128
	visitorAuditMaxUserAgentLen = 512
	visitorAuditMaxDetailLen    = 2048
	visitorAuditMaxMessageLen   = 4096
	visitorAuditRatePerMinute   = 30
	visitorAuditRateBurst       = 10
	visitorAuditRateMaxEntries  = 10000
	visitorAuditLimiterEntryTTL = 10 * time.Minute
	visitorAuditCleanupInterval = time.Minute
	visitorAuditMessagePrefix   = "visitor event: "
	visitorAuditUnknownIPKey    = "<unknown>"
)

// ErrVisitorEventRequired is returned by RecordVisitorEvent when the event
// name is empty or contains no characters the sanitizer allows.
var ErrVisitorEventRequired = errors.New("event is required")

type visitorAuditRateState struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

type visitorAuditRateLimiter struct {
	mu          sync.Mutex
	entries     map[string]*visitorAuditRateState
	lastCleanup time.Time
}

var visitorAuditLimiter = newVisitorAuditRateLimiter()

// VisitorEvent describes one frontend visitor telemetry event pending
// sanitization and persistence.
type VisitorEvent struct {
	IP        string
	UUID      string
	UserAgent string
	Event     string
	Path      string
	Route     string
	Target    string
	Detail    map[string]any
}

type visitorAuditMessage struct {
	Event     string         `json:"event"`
	Path      string         `json:"path,omitempty"`
	Route     string         `json:"route,omitempty"`
	Target    string         `json:"target,omitempty"`
	UserAgent string         `json:"user_agent,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// AllowVisitorEvent reports whether a visitor event from ip passes the
// per-IP rate limit (30/min, burst of 10). Callers should check this before
// doing any other work for the request.
func AllowVisitorEvent(ip string) bool {
	return visitorAuditLimiter.Allow(ip, time.Now())
}

// VisitorAuditEnabled reports whether visitor event recording is turned on.
func VisitorAuditEnabled() (bool, error) {
	return kv.GetAs[bool](settings.VisitorAuditEnabledKey, false)
}

// RecordVisitorEvent normalizes and bounds ev, then persists it as an audit
// log entry of type "visitor". Callers should have already checked
// AllowVisitorEvent and VisitorAuditEnabled.
func RecordVisitorEvent(ev VisitorEvent) error {
	event := normalizeVisitorAuditEvent(ev.Event)
	if event == "" {
		return ErrVisitorEventRequired
	}

	message, err := buildVisitorAuditMessage(visitorAuditMessage{
		Event:     event,
		Path:      strings.TrimSpace(ev.Path),
		Route:     strings.TrimSpace(ev.Route),
		Target:    strings.TrimSpace(ev.Target),
		UserAgent: strings.TrimSpace(ev.UserAgent),
		Detail:    ev.Detail,
	})
	if err != nil {
		return err
	}

	Log(ev.IP, ev.UUID, message, "visitor")
	return nil
}

func newVisitorAuditRateLimiter() *visitorAuditRateLimiter {
	return &visitorAuditRateLimiter{entries: make(map[string]*visitorAuditRateState)}
}

func (l *visitorAuditRateLimiter) Allow(ip string, now time.Time) bool {
	key := strings.TrimSpace(ip)
	if key == "" {
		key = visitorAuditUnknownIPKey
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= visitorAuditCleanupInterval {
		l.cleanupLocked(now)
	}

	state, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= visitorAuditRateMaxEntries {
			return false
		}
		l.entries[key] = &visitorAuditRateState{
			tokens:     visitorAuditRateBurst - 1,
			lastRefill: now,
			lastSeen:   now,
		}
		return true
	}

	if now.After(state.lastRefill) {
		elapsedMinutes := now.Sub(state.lastRefill).Minutes()
		state.tokens = min(float64(visitorAuditRateBurst), state.tokens+elapsedMinutes*visitorAuditRatePerMinute)
		state.lastRefill = now
	}
	state.lastSeen = now
	if state.tokens < 1 {
		return false
	}
	state.tokens--
	return true
}

func (l *visitorAuditRateLimiter) cleanupLocked(now time.Time) {
	for key, state := range l.entries {
		if now.Sub(state.lastSeen) >= visitorAuditLimiterEntryTTL {
			delete(l.entries, key)
		}
	}
	l.lastCleanup = now
}

func normalizeVisitorAuditEvent(event string) string {
	event = strings.TrimSpace(strings.ToLower(event))
	if event == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(event))
	written := 0
	for _, r := range event {
		allowed := false
		switch {
		case r == '_' || r == '-' || r == ':' || r == '.':
			allowed = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			allowed = true
		case unicode.IsSpace(r):
			r = '_'
			allowed = true
		}
		if !allowed {
			continue
		}
		if written >= visitorAuditMaxEventLen {
			break
		}
		builder.WriteRune(r)
		written++
	}
	return builder.String()
}

func trimVisitorAuditDetail(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return nil
	}
	if len(encoded) <= visitorAuditMaxDetailLen {
		return detail
	}
	return map[string]any{"truncated": true, "size": len(encoded)}
}

func buildVisitorAuditMessage(message visitorAuditMessage) (string, error) {
	message.Event = truncateString(strings.TrimSpace(message.Event), visitorAuditMaxEventLen)
	message.Path = truncateString(strings.TrimSpace(message.Path), visitorAuditMaxPathLen)
	message.Route = truncateString(strings.TrimSpace(message.Route), visitorAuditMaxRouteLen)
	message.Target = truncateString(strings.TrimSpace(message.Target), visitorAuditMaxTargetLen)
	message.UserAgent = truncateString(strings.TrimSpace(message.UserAgent), visitorAuditMaxUserAgentLen)
	message.Detail = trimVisitorAuditDetail(message.Detail)

	detailReduced := false
	for {
		encoded, err := json.Marshal(message)
		if err != nil {
			return "", err
		}
		if len(visitorAuditMessagePrefix)+len(encoded) <= visitorAuditMaxMessageLen {
			return visitorAuditMessagePrefix + string(encoded), nil
		}

		if message.Detail != nil && !detailReduced {
			message.Detail = map[string]any{"truncated": true}
			detailReduced = true
			continue
		}
		if !shrinkLongestVisitorAuditField(&message) {
			return "", errors.New("visitor audit message exceeds maximum length")
		}
	}
}

func shrinkLongestVisitorAuditField(message *visitorAuditMessage) bool {
	fields := []*string{&message.Path, &message.UserAgent, &message.Route, &message.Target}
	var longest *string
	longestLen := 0
	for _, field := range fields {
		if size := utf8.RuneCountInString(*field); size > longestLen {
			longest = field
			longestLen = size
		}
	}
	if longest == nil {
		return false
	}
	*longest = truncateString(*longest, longestLen/2)
	return true
}

func truncateString(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
