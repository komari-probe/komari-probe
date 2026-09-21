package ping

import (
	"sort"
	"testing"
)

func TestPercentileLatencies(t *testing.T) {
	values := []int{5, 1, 3, 2, 4, 100, 90, 80, 70, 60}
	sort.Ints(values)
	p50, p99 := PercentileLatencies(values)
	// sorted: 1 2 3 4 5 60 70 80 90 100 (n=10); pos(0.50)=4.5 -> interpolate
	// vals[4]=5 and vals[5]=60; pos(0.99)=8.91 -> interpolate vals[8]=90 and vals[9]=100.
	if p50 != 33 {
		t.Fatalf("p50 = %d, want 33", p50)
	}
	if p99 != 99 {
		t.Fatalf("p99 = %d, want 99", p99)
	}

	if p50, p99 := PercentileLatencies(nil); p50 != 0 || p99 != 0 {
		t.Fatalf("empty input should return zeros, got p50=%d p99=%d", p50, p99)
	}
}

func TestVolatility(t *testing.T) {
	// Small P50 must not blow the ratio up: denominator floors at 10.
	if ratio, ok := Volatility(2, 20); !ok || ratio != 1.8 {
		t.Fatalf("Volatility(2, 20) = (%v, %v), want (1.8, true)", ratio, ok)
	}
	// Large P50 must not dilute the ratio to near zero: denominator caps at 50.
	if ratio, ok := Volatility(200, 250); !ok || ratio != 1.0 {
		t.Fatalf("Volatility(200, 250) = (%v, %v), want (1.0, true)", ratio, ok)
	}
	if _, ok := Volatility(0, 10); ok {
		t.Fatal("p50 <= 0 must report ok=false")
	}
	if _, ok := Volatility(50, 40); ok {
		t.Fatal("p99 < p50 must report ok=false")
	}
}
