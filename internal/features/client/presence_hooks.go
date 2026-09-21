package client

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
		hook(uuid, connID)
	}
}

func notifyOffline(uuid string, connID int64) {
	for _, hook := range offlineHooks {
		hook(uuid, connID)
	}
}
