package metricruntime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/sonar-probe/sonar/pkg/kv"
	"github.com/sonar-probe/sonar/pkg/lifecycle"
)

// configKeys are the settings keys that belong to the metric store's own
// configuration and rollup retention policy.
var configKeys = map[string]struct{}{
	MetricDBDriverKey:                         {},
	MetricDBDSNKey:                            {},
	MetricTablePrefixKey:                      {},
	MetricMaxOpenConnsKey:                     {},
	MetricMaxIdleConnsKey:                     {},
	MetricRollupMinuteRetentionMinutesKey:     {},
	MetricRollupFiveMinuteRetentionMinutesKey: {},
	MetricRollupHourRetentionHoursKey:         {},
}

// ConfigKeysTouched reports whether cfg contains any key belonging to the
// metric store's own configuration.
func ConfigKeysTouched(cfg map[string]any) bool {
	for key := range cfg {
		if _, ok := configKeys[key]; ok {
			return true
		}
	}
	return false
}

// ValidateRollupSettingChanges rejects non-positive rollup retention values
// among the keys present in cfg. Keys absent from cfg are left unchecked.
func ValidateRollupSettingChanges(cfg map[string]any) error {
	keys := []string{
		MetricRollupMinuteRetentionMinutesKey,
		MetricRollupFiveMinuteRetentionMinutesKey,
		MetricRollupHourRetentionHoursKey,
	}
	for _, key := range keys {
		value, ok := cfg[key]
		if !ok {
			continue
		}
		n, err := rollupSettingInt(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("%s must be a positive integer", key)
		}
	}
	return nil
}

func rollupSettingInt(value any) (int, error) {
	maxInt := float64(^uint(0) >> 1)
	switch value := value.(type) {
	case int:
		return value, nil
	case int8:
		return int(value), nil
	case int16:
		return int(value), nil
	case int32:
		return int(value), nil
	case int64:
		if float64(value) > maxInt || float64(value) < -maxInt-1 {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case uint:
		if float64(value) > maxInt {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case uint8:
		return int(value), nil
	case uint16:
		return int(value), nil
	case uint32:
		if float64(value) > maxInt {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case uint64:
		if float64(value) > maxInt {
			return 0, fmt.Errorf("integer overflow")
		}
		return int(value), nil
	case float32:
		return rollupSettingInt(float64(value))
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value > maxInt || value < -maxInt-1 {
			return 0, fmt.Errorf("not an integer")
		}
		return int(value), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(value))
	default:
		return 0, fmt.Errorf("unsupported numeric type")
	}
}

// toInt converts a JSON-decoded value (typically float64 or string) to int,
// returning fallback on failure.
func toInt(v any, fallback int) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}

// NormalizeAndMergeConfig infers metric_db_driver from a metric_db_dsn
// present in cfg (writing it back into cfg so the caller persists a
// consistent pair), then merges cfg's metric-store keys onto the currently
// persisted configuration to produce the config that is about to take
// effect. Callers use the result to test-connect before persisting cfg.
func NormalizeAndMergeConfig(cfg map[string]any) (*MetricStoreConfig, error) {
	if v, ok := cfg[MetricDBDSNKey]; ok {
		if dsn, ok := v.(string); ok {
			dsn = strings.TrimSpace(dsn)
			cfg[MetricDBDSNKey] = dsn
			if driver, inferred := InferDriverFromDSN(dsn); inferred {
				cfg[MetricDBDriverKey] = string(driver)
			}
		}
	}

	merged, err := kv.GetManyAs[MetricStoreConfig]()
	if err != nil {
		return nil, err
	}

	if v, ok := cfg[MetricDBDriverKey]; ok {
		if s, ok := v.(string); ok {
			merged.Driver = s
		}
	}
	if v, ok := cfg[MetricDBDSNKey]; ok {
		if s, ok := v.(string); ok {
			merged.DSN = s
		}
	}
	if v, ok := cfg[MetricTablePrefixKey]; ok {
		if s, ok := v.(string); ok {
			merged.TablePrefix = s
		}
	}
	if v, ok := cfg[MetricMaxOpenConnsKey]; ok {
		merged.MaxOpenConns = toInt(v, merged.MaxOpenConns)
	}
	if v, ok := cfg[MetricMaxIdleConnsKey]; ok {
		merged.MaxIdleConns = toInt(v, merged.MaxIdleConns)
	}
	if v, ok := cfg[MetricRollupMinuteRetentionMinutesKey]; ok {
		merged.RollupMinuteRetentionMinutes = toInt(v, merged.RollupMinuteRetentionMinutes)
	}
	if v, ok := cfg[MetricRollupFiveMinuteRetentionMinutesKey]; ok {
		merged.RollupFiveMinuteRetentionMinutes = toInt(v, merged.RollupFiveMinuteRetentionMinutes)
	}
	if v, ok := cfg[MetricRollupHourRetentionHoursKey]; ok {
		merged.RollupHourRetentionHours = toInt(v, merged.RollupHourRetentionHours)
	}

	return merged, nil
}

// ConfigChangeOutcome describes what applying an already-persisted
// metric-store config change ended up requiring.
type ConfigChangeOutcome struct {
	RestartRequired bool
	GuidePath       string
}

// ReloadAfterConfigChange hot-reloads the metric store after cfg has already
// been persisted. It is a no-op (zero outcome, nil error) when cfg touches
// no metric-store key. When the new configuration needs a structural
// upgrade that cannot happen while the normal HTTP server keeps running, it
// requests a restart into the migration guide instead of returning an error.
func ReloadAfterConfigChange(ctx context.Context, cfg map[string]any) (ConfigChangeOutcome, error) {
	if !ConfigKeysTouched(cfg) {
		return ConfigChangeOutcome{}, nil
	}
	reloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := Reload(reloadCtx); err != nil {
		if errors.Is(err, ErrStructureUpgradeRequired) {
			lifecycle.RequestRestart(lifecycle.RestartForMetricStoreStructureUpgrade)
			return ConfigChangeOutcome{RestartRequired: true, GuidePath: "/admin/database-migration"}, nil
		}
		return ConfigChangeOutcome{}, err
	}
	return ConfigChangeOutcome{}, nil
}
