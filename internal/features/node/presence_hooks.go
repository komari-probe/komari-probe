package node

import "github.com/sonar-probe/sonar/pkg/logger"

// PresenceHook is invoked when a client's connectivity state changes.
// It exists so this package doesn't need to import internal/features/notification
// directly: notification already depends on this package for connection/report
// state, so the reverse edge is registered here instead of imported there.
type PresenceHook func(uuid string, connID int64)

var (
	onlineHooks  []PresenceHook
	offlineHooks []PresenceHook
)

// OnOnline registers a hook to run whenever a client comes online.
func OnOnline(hook PresenceHook) {
	onlineHooks = append(onlineHooks, hook)
}

// OnOffline registers a hook to run whenever a client goes offline.
func OnOffline(hook PresenceHook) {
	offlineHooks = append(offlineHooks, hook)
}

func notifyOnline(uuid string, connID int64) {
	for _, hook := range onlineHooks {
		callPresenceHookSafely(hook, uuid, connID)
	}
}

func notifyOffline(uuid string, connID int64) {
	for _, hook := range offlineHooks {
		callPresenceHookSafely(hook, uuid, connID)
	}
}

// callPresenceHookSafely recovers a panicking hook so one misbehaving
// registrant can't take down the process or block the other hooks. Some
// callers (the HTTP POST presence path) previously invoked notifyOnline/
// notifyOffline with no recovery at all; the safety belongs here, not in
// each caller.
func callPresenceHookSafely(hook PresenceHook, uuid string, connID int64) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("node", "presence hook panicked for client %s: %v", uuid, r)
		}
	}()
	hook(uuid, connID)
}
