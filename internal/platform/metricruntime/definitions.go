package metricruntime

import (
	"context"
	"errors"
	"fmt"

	"github.com/sonar-probe/sonar/pkg/tsdb"
)

const defaultBuiltinMetricRetentionDays = 1

// RetentionSummary is the compatibility view of all persisted metric policies.
type RetentionSummary struct {
	AllPositive bool
	MaxDays     int
}

// GetRetentionSummary aggregates the active store's metric definitions. An
// empty definition set is not considered record-enabled.
func GetRetentionSummary(ctx context.Context) (RetentionSummary, error) {
	s := GetStore()
	if s == nil {
		return RetentionSummary{}, fmt.Errorf("metric store not initialized")
	}
	defs, err := s.ListMetrics(ctx)
	if err != nil {
		return RetentionSummary{}, err
	}
	return summarizeRetentionDefinitions(defs), nil
}

func summarizeRetentionDefinitions(defs []tsdb.Definition) RetentionSummary {
	if len(defs) == 0 {
		return RetentionSummary{}
	}
	summary := RetentionSummary{AllPositive: true}
	for _, def := range defs {
		if def.RetentionDays <= 0 {
			summary.AllPositive = false
		}
		if def.RetentionDays > summary.MaxDays {
			summary.MaxDays = def.RetentionDays
		}
	}
	return summary
}

// createMetricDefinitions creates built-in definitions with explicit policies.
func createMetricDefinitions(ctx context.Context, s *tsdb.Store) error {
	return createMetricDefinitionsWithDefaultRetention(ctx, s, defaultBuiltinMetricRetentionDays)
}

// EnsureBuiltinMetricDefinitions registers definitions for the server's
// built-in report and ping writers before a standalone Store receives points.
func EnsureBuiltinMetricDefinitions(ctx context.Context, s *tsdb.Store) error {
	return createMetricDefinitions(ctx, s)
}

func createMetricDefinitionsWithDefaultRetention(ctx context.Context, s *tsdb.Store, defaultRetentionDays int) error {
	if defaultRetentionDays < defaultBuiltinMetricRetentionDays {
		defaultRetentionDays = defaultBuiltinMetricRetentionDays
	}
	definitions := []tsdb.Definition{
		{Name: MetricCPU, Type: tsdb.TypeGauge, Unit: "%", Description: "CPU usage percentage", RetentionDays: defaultRetentionDays},
		{Name: MetricGPU, Type: tsdb.TypeGauge, Unit: "%", Description: "GPU usage percentage", RetentionDays: defaultRetentionDays},
		{Name: MetricGPUDeviceUsage, Type: tsdb.TypeGauge, Unit: "%", Description: "Per-device GPU utilization", RetentionDays: defaultRetentionDays},
		{Name: MetricGPUMem, Type: tsdb.TypeGauge, Unit: "bytes", Description: "GPU memory used", RetentionDays: defaultRetentionDays},
		{Name: MetricGPUMemTotal, Type: tsdb.TypeGauge, Unit: "bytes", Description: "GPU memory total", RetentionDays: defaultRetentionDays},
		{Name: MetricGPUTemp, Type: tsdb.TypeGauge, Unit: "°C", Description: "GPU temperature", RetentionDays: defaultRetentionDays},
		{Name: MetricRAM, Type: tsdb.TypeGauge, Unit: "bytes", Description: "RAM used", RetentionDays: defaultRetentionDays},
		{Name: MetricSwap, Type: tsdb.TypeGauge, Unit: "bytes", Description: "Swap used", RetentionDays: defaultRetentionDays},
		{Name: MetricLoad, Type: tsdb.TypeGauge, Unit: "", Description: "System load average", RetentionDays: defaultRetentionDays},
		{Name: MetricDisk, Type: tsdb.TypeGauge, Unit: "bytes", Description: "Disk used", RetentionDays: defaultRetentionDays},
		{Name: MetricNetIn, Type: tsdb.TypeGauge, Unit: "bytes/s", Description: "Network in rate", RetentionDays: defaultRetentionDays},
		{Name: MetricNetOut, Type: tsdb.TypeGauge, Unit: "bytes/s", Description: "Network out rate", RetentionDays: defaultRetentionDays},
		{Name: MetricNetTotalUp, Type: tsdb.TypeCounter, Unit: "bytes", Description: "Network total upload", RetentionDays: defaultRetentionDays},
		{Name: MetricNetTotalDown, Type: tsdb.TypeCounter, Unit: "bytes", Description: "Network total download", RetentionDays: defaultRetentionDays},
		{Name: MetricTrafficUp, Type: tsdb.TypeGauge, Unit: "bytes", Description: "Traffic upload delta", RetentionDays: defaultRetentionDays},
		{Name: MetricTrafficDown, Type: tsdb.TypeGauge, Unit: "bytes", Description: "Traffic download delta", RetentionDays: defaultRetentionDays},
		{Name: MetricProcess, Type: tsdb.TypeGauge, Unit: "count", Description: "Process count", RetentionDays: defaultRetentionDays},
		{Name: MetricConnections, Type: tsdb.TypeGauge, Unit: "count", Description: "TCP connections", RetentionDays: defaultRetentionDays},
		{Name: MetricConnectionsUDP, Type: tsdb.TypeGauge, Unit: "count", Description: "UDP connections", RetentionDays: defaultRetentionDays},
		{Name: MetricPingLatency, Type: tsdb.TypeGauge, Unit: "ms", Description: "Ping latency", RetentionDays: defaultRetentionDays},
		{Name: MetricPingLoss, Type: tsdb.TypeGauge, Unit: "ratio", Description: "Ping packet loss indicator", RetentionDays: defaultRetentionDays},
	}

	for _, def := range definitions {
		existing, err := s.GetMetric(ctx, def.Name)
		if err != nil && !errors.Is(err, tsdb.ErrNotFound) {
			return fmt.Errorf("failed to get metric %s: %w", def.Name, err)
		}
		if err == nil {
			if existing.RetentionDays == 0 {
				continue
			}
			def.RetentionDays = existing.RetentionDays
		}
		if err := s.UpsertMetric(ctx, def); err != nil {
			return fmt.Errorf("failed to create metric %s: %w", def.Name, err)
		}
	}
	return nil
}
