package node

import "github.com/sonar-probe/sonar/internal/platform/models"

// This package and features/ping naturally depend on each other (ping needs
// to dispatch commands to connected agents; client needs to persist reported
// ping results and seed default-on tasks for new clients), so importing
// features/ping directly here would create an import cycle. ping registers
// these hooks from its own init() instead.

var (
	pingResultRecorder     func(models.PingRecord) error
	defaultPingTaskApplier func(clientUUID string) error
)

// SetPingResultRecorder registers the function used to persist a ping
// result reported by an agent.
func SetPingResultRecorder(fn func(models.PingRecord) error) {
	pingResultRecorder = fn
}

// SetDefaultPingTaskApplier registers the function used to apply
// default-on ping tasks to a newly created client.
func SetDefaultPingTaskApplier(fn func(clientUUID string) error) {
	defaultPingTaskApplier = fn
}
