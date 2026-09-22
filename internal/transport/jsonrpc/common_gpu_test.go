package jsonrpc

import (
	"testing"

	"github.com/sonar-probe/sonar/internal/platform/protocol"
)

func TestGpuUsageFromReport(t *testing.T) {
	if gpuUsageFromReport(nil) != 0 {
		t.Fatalf("nil report should be 0")
	}
	if gpuUsageFromReport(&protocol.Report{}) != 0 {
		t.Fatalf("missing GPU should be 0")
	}

	got := gpuUsageFromReport(&protocol.Report{
		GPU: &protocol.GPUDetailReport{
			Count:        1,
			AverageUsage: 87.5,
			DetailedInfo: []protocol.GPUDeviceInfo{{
				Name:        "Phoenix1",
				Utilization: 87.5,
			}},
		},
	})
	if got != 87.5 {
		t.Fatalf("got %v, want 87.5", got)
	}
}
