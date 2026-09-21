package tsdb

import (
	"testing"
	"time"
)

func TestStandardIntervals(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		floor    time.Duration
		ceil     time.Duration
	}{
		{name: "below minimum", interval: 500 * time.Millisecond, floor: time.Second, ceil: time.Second},
		{name: "exact", interval: 5 * time.Minute, floor: 5 * time.Minute, ceil: 5 * time.Minute},
		{name: "between standards", interval: 7 * time.Minute, floor: 5 * time.Minute, ceil: 10 * time.Minute},
		{name: "above one day", interval: 25 * time.Hour, floor: 24 * time.Hour, ceil: 48 * time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FloorStandardInterval(test.interval); got != test.floor {
				t.Fatalf("FloorStandardInterval(%s) = %s, want %s", test.interval, got, test.floor)
			}
			if got := CeilStandardInterval(test.interval); got != test.ceil {
				t.Fatalf("CeilStandardInterval(%s) = %s, want %s", test.interval, got, test.ceil)
			}
		})
	}
}

func TestInferCollectionInterval(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }

	// Too few points to estimate a cadence: known passes through unchanged.
	if got := InferCollectionInterval(nil, 30*time.Second); got != 30*time.Second {
		t.Fatalf("no points: got %s, want 30s", got)
	}
	if got := InferCollectionInterval([]time.Time{at(0), at(30)}, 30*time.Second); got != 30*time.Second {
		t.Fatalf("single delta: got %s, want 30s", got)
	}

	// A regular 30s cadence must be picked up even when known is smaller.
	regular := []time.Time{at(0), at(30), at(60), at(90), at(120)}
	if got := InferCollectionInterval(regular, 10*time.Second); got != 30*time.Second {
		t.Fatalf("regular cadence: got %s, want 30s", got)
	}

	// One long outage must not inflate the inferred interval above the
	// series' normal cadence (lower-quartile smoothing).
	withOutage := []time.Time{at(0), at(30), at(60), at(700), at(730)}
	if got := InferCollectionInterval(withOutage, 10*time.Second); got != 30*time.Second {
		t.Fatalf("one outage: got %s, want 30s", got)
	}

	// known must never be lowered below its input value.
	if got := InferCollectionInterval(regular, time.Minute); got != time.Minute {
		t.Fatalf("known floor: got %s, want 1m", got)
	}
}
