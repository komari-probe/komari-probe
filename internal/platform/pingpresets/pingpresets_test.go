package pingpresets

import "testing"

func TestProvinceCount(t *testing.T) {
	if len(Provinces) != 31 {
		t.Fatalf("expected 31 provinces, got %d", len(Provinces))
	}
}

func TestCarrierCount(t *testing.T) {
	if len(Carriers) != 3 {
		t.Fatalf("expected 3 carriers, got %d", len(Carriers))
	}
}

func TestNodesCountAndUnique(t *testing.T) {
	for _, v := range []int{4, 6} {
		nodes := Nodes(v)
		if len(nodes) != 93 {
			t.Fatalf("ip v%d: expected 93 nodes, got %d", v, len(nodes))
		}
		seen := make(map[string]bool, len(nodes))
		for _, n := range nodes {
			if seen[n.Target] {
				t.Fatalf("duplicate target %q", n.Target)
			}
			seen[n.Target] = true
			if n.IPVersion != v {
				t.Fatalf("node %q has ip version %d, want %d", n.Target, n.IPVersion, v)
			}
		}
	}
}

func TestAllNodesCount(t *testing.T) {
	if got := len(AllNodes()); got != 186 {
		t.Fatalf("expected 186 nodes total, got %d", got)
	}
}

func TestNoDuplicateProvinceCodes(t *testing.T) {
	seen := make(map[string]bool, len(Provinces))
	for _, p := range Provinces {
		if seen[p.Code] {
			t.Fatalf("duplicate province code %q", p.Code)
		}
		seen[p.Code] = true
	}
}
