package downsample

import (
	"math"
	"sort"
)

// AllocationGroup is one group of records competing for a share of a fixed
// total point budget in AllocateTargets.
type AllocationGroup[K comparable] struct {
	Key    K
	Length int
}

// AllocateTargets splits maxTotal across groups proportionally to their
// lengths, using largest-remainder rounding so the allocation sums exactly
// to maxTotal (or to the sum of group lengths, whichever is smaller). A
// negative maxTotal means "unlimited": every group keeps its full length.
func AllocateTargets[K comparable](groups []AllocationGroup[K], maxTotal int) map[K]int {
	result := make(map[K]int, len(groups))
	if maxTotal < 0 {
		for _, g := range groups {
			result[g.Key] = g.Length
		}
		return result
	}
	total := 0
	for _, g := range groups {
		total += g.Length
	}
	if total <= maxTotal {
		for _, g := range groups {
			result[g.Key] = g.Length
		}
		return result
	}
	// initial allocation based on proportion
	type rem struct {
		idx  int
		frac float64
	}
	remainders := make([]rem, 0, len(groups))
	sumTargets := 0
	for i, g := range groups {
		if g.Length <= 0 {
			result[g.Key] = 0
			continue
		}
		raw := float64(g.Length) * float64(maxTotal) / float64(total)
		t := int(math.Floor(raw))
		if t > g.Length {
			t = g.Length
		}
		result[groups[i].Key] = t
		sumTargets += t
		remainders = append(remainders, rem{i, raw - float64(t)})
	}
	// distribute remaining by largest fractional parts
	if sumTargets < maxTotal {
		need := maxTotal - sumTargets
		sort.Slice(remainders, func(i, j int) bool { return remainders[i].frac > remainders[j].frac })
		for _, r := range remainders {
			if need == 0 {
				break
			}
			g := groups[r.idx]
			cur := result[g.Key]
			if cur < g.Length {
				result[g.Key] = cur + 1
				need--
			}
		}
		// if still left, second pass round-robin
		if need > 0 {
			for need > 0 {
				for _, g := range groups {
					if need == 0 {
						break
					}
					if result[g.Key] < g.Length {
						result[g.Key]++
						need--
					}
				}
				if need > 0 {
					break
				}
			}
		}
	} else if sumTargets > maxTotal {
		over := sumTargets - maxTotal
		sort.Slice(remainders, func(i, j int) bool { return remainders[i].frac < remainders[j].frac })
		for _, r := range remainders {
			if over == 0 {
				break
			}
			g := groups[r.idx]
			if result[g.Key] > 0 {
				result[g.Key]--
				over--
			}
		}
		if over > 0 {
			for over > 0 {
				for _, g := range groups {
					if over == 0 {
						break
					}
					if result[g.Key] > 0 {
						result[g.Key]--
						over--
					}
				}
				if over > 0 {
					break
				}
			}
		}
	}
	return result
}

// SampleEvenly returns k elements of in, spaced as evenly as possible across
// its full span (always including the first and last element when k >= 2).
// It returns in unchanged when k >= len(in), and an empty slice when k <= 0.
func SampleEvenly[T any](in []T, k int) []T {
	n := len(in)
	if k <= 0 || n == 0 {
		return []T{}
	}
	if k >= n {
		return in
	}
	out := make([]T, 0, k)
	if k == 1 {
		out = append(out, in[n-1])
		return out
	}
	for i := 0; i < k; i++ {
		idx := int(math.Round(float64(i) * float64(n-1) / float64(k-1)))
		if idx < 0 {
			idx = 0
		} else if idx >= n {
			idx = n - 1
		}
		out = append(out, in[idx])
	}
	return out
}
