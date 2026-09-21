package ping

import "math"

// MinSamplesForVolatility is the minimum number of valid (non-loss) latency
// readings a window needs before a P50/P99-derived volatility ratio is
// considered meaningful. Below this, percentiles are dominated by noise
// (e.g. a P99 of 10 samples is essentially just the 2nd-largest reading).
const MinSamplesForVolatility = 20

// PercentileLatencies returns the 50th and 99th percentile of sortedValues
// using linear interpolation between the two nearest ranks. sortedValues
// must already be sorted ascending.
func PercentileLatencies(sortedValues []int) (p50, p99 int) {
	if len(sortedValues) == 0 {
		return 0, 0
	}
	percentile := func(pct float64) int {
		if pct <= 0 {
			return sortedValues[0]
		}
		if pct >= 1 {
			return sortedValues[len(sortedValues)-1]
		}
		pos := float64(len(sortedValues)-1) * pct
		lo := int(math.Floor(pos))
		hi := int(math.Ceil(pos))
		if lo == hi {
			return sortedValues[lo]
		}
		frac := pos - float64(lo)
		v := float64(sortedValues[lo]) + (float64(sortedValues[hi])-float64(sortedValues[lo]))*frac
		return int(math.Round(v))
	}
	return percentile(0.50), percentile(0.99)
}

// Volatility measures how much worse the tail (P99) is than the median
// (P50): (P99-P50) divided by a base clamped to [10, 50] so a very small P50
// doesn't blow the ratio up out of proportion. This is a product-defined
// "how spiky is this task's latency" indicator (labeled "Volatility"/"波动"
// in the UI) -- it is not network jitter in the RFC 3550 sense. ok is false
// when p50/p99 don't support a meaningful ratio (no samples, or p99 < p50).
func Volatility(p50, p99 float64) (ratio float64, ok bool) {
	if p50 <= 0 || p99 < p50 {
		return 0, false
	}
	base := math.Max(math.Min(p50, 50.0), 10.0)
	return (p99 - p50) / base, true
}
