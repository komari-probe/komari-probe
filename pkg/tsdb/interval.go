package tsdb

import (
	"sort"
	"time"
)

var standardQueryIntervals = [...]time.Duration{
	time.Second,
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
	30 * time.Second,
	time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	time.Hour,
	2 * time.Hour,
	3 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
}

// FloorStandardInterval returns the largest standard query interval that does
// not exceed interval. Values below one second use the minimum interval.
func FloorStandardInterval(interval time.Duration) time.Duration {
	selected := standardQueryIntervals[0]
	for _, candidate := range standardQueryIntervals {
		if candidate > interval {
			break
		}
		selected = candidate
	}
	return selected
}

// CeilStandardInterval returns the smallest standard query interval that is
// at least interval. Values above one day are rounded up to whole days.
func CeilStandardInterval(interval time.Duration) time.Duration {
	for _, candidate := range standardQueryIntervals {
		if candidate >= interval {
			return candidate
		}
	}
	day := standardQueryIntervals[len(standardQueryIntervals)-1]
	return ((interval-1)/day + 1) * day
}

// InferCollectionInterval estimates a series' collection cadence from its
// point timestamps (sorted or not; only the multiset of consecutive gaps
// matters), using the lower quartile of observed deltas so a handful of
// outages don't inflate the inferred interval above the series' normal
// cadence. known is a previously-known interval (e.g. a metric definition's
// configured collection interval); the result is never smaller than known,
// and known itself is returned when there aren't enough points to estimate
// a cadence.
func InferCollectionInterval(times []time.Time, known time.Duration) time.Duration {
	if len(times) < 2 {
		return known
	}
	deltas := make([]time.Duration, 0, len(times)-1)
	for i := 1; i < len(times); i++ {
		if delta := times[i].Sub(times[i-1]); delta > 0 {
			deltas = append(deltas, delta)
		}
	}
	// Two deltas are the minimum needed to distinguish a regular cadence
	// from one isolated long gap.
	if len(deltas) < 2 {
		return known
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i] < deltas[j] })
	observed := deltas[(len(deltas)-1)/4]
	if observed > known {
		return observed
	}
	return known
}
