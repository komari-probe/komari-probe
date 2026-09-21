package downsample

import (
	"reflect"
	"testing"
)

func TestSampleEvenly(t *testing.T) {
	input := []int{0, 1, 2, 3, 4}
	tests := []struct {
		name  string
		count int
		want  []int
	}{
		{name: "empty", count: 0, want: []int{}},
		{name: "latest only", count: 1, want: []int{4}},
		{name: "even selection", count: 3, want: []int{0, 2, 4}},
		{name: "all", count: len(input), want: input},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SampleEvenly(input, test.count); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("SampleEvenly(%v, %d) = %v, want %v", input, test.count, got, test.want)
			}
		})
	}
}

func TestAllocateTargetsSupportsTypedKeys(t *testing.T) {
	groups := []AllocationGroup[uint]{
		{Key: 7, Length: 6},
		{Key: 9, Length: 4},
	}
	got := AllocateTargets(groups, 5)
	if got[7] != 3 || got[9] != 2 {
		t.Fatalf("AllocateTargets() = %v, want map[7:3 9:2]", got)
	}
}
