package scheduler

import (
	"testing"
	"time"
)

func TestCronScheduleUsesSystemLocalWallClock(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = originalLocal })

	schedule, err := Parse("0 0 9 * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}
	after := time.Date(2026, 7, 17, 0, 30, 0, 0, time.UTC)
	want := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	if got := schedule.Next(after); !got.Equal(want) {
		t.Fatalf("next run = %s, want %s", got, want)
	} else if got.Location() != time.UTC {
		t.Fatalf("next run location = %s, want UTC", got.Location())
	}
}

func TestEverySchedulePreservesElapsedDuration(t *testing.T) {
	schedule, err := Parse("@every 90s")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}
	after := time.Now()
	if got := schedule.Next(after); got.Sub(after) != 90*time.Second {
		t.Fatalf("interval = %s, want 90s", got.Sub(after))
	}
}

func TestParseFieldBareValueWithStep(t *testing.T) {
	sched, err := Parse("5/10 * * * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}
	cs, ok := sched.(cronSchedule)
	if !ok {
		t.Fatalf("expected cronSchedule, got %T", sched)
	}
	want := map[int]struct{}{5: {}, 15: {}, 25: {}, 35: {}, 45: {}, 55: {}}
	if len(cs.seconds) != len(want) {
		t.Fatalf("seconds = %v, want %v", cs.seconds, want)
	}
	for k := range want {
		if _, ok := cs.seconds[k]; !ok {
			t.Fatalf("seconds missing %d: %v", k, cs.seconds)
		}
	}
}

func TestAddFuncRejectsScheduleThatNeverMatches(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.StopAll)

	err := m.AddFunc("impossible", "0 0 0 30 2 *", func() {})
	if err == nil {
		t.Fatal("expected error for a spec that never matches (Feb 30), got nil")
	}
	if _, ok := m.jobs["impossible"]; ok {
		t.Fatal("job should not be registered when its schedule never matches")
	}
}
