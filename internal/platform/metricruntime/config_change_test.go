package metricruntime

import "testing"

func TestConfigKeysTouched(t *testing.T) {
	if !ConfigKeysTouched(map[string]interface{}{MetricDBDSNKey: "metrics.db"}) {
		t.Fatal("metric database DSN must trigger metric store validation")
	}
	for _, key := range []string{
		MetricRollupMinuteRetentionMinutesKey,
		MetricRollupFiveMinuteRetentionMinutesKey,
		MetricRollupHourRetentionHoursKey,
	} {
		if !ConfigKeysTouched(map[string]interface{}{key: 1}) {
			t.Fatalf("%s must trigger metric store validation", key)
		}
	}
}

func TestValidateRollupSettingChanges(t *testing.T) {
	if err := ValidateRollupSettingChanges(map[string]interface{}{
		MetricRollupMinuteRetentionMinutesKey:     float64(30),
		MetricRollupFiveMinuteRetentionMinutesKey: float64(150),
		MetricRollupHourRetentionHoursKey:         float64(300),
	}); err != nil {
		t.Fatalf("valid rollup settings rejected: %v", err)
	}

	for _, value := range []interface{}{float64(0), float64(-1), float64(1.5), "not-a-number"} {
		err := ValidateRollupSettingChanges(map[string]interface{}{
			MetricRollupMinuteRetentionMinutesKey: value,
		})
		if err == nil {
			t.Fatalf("value %#v should be rejected", value)
		}
	}
}
