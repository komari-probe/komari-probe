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

// bruteForceNext 是重写前 Next() 的逐秒枚举实现，仅用于测试里交叉验证按字段
// 跳跃算法的正确性；不用于生产代码。
func bruteForceNext(s cronSchedule, t time.Time) time.Time {
	next := t.In(time.Local).Truncate(time.Second).Add(time.Second)
	limit := next.Add(366 * 24 * time.Hour)
	for next.Before(limit) {
		_, okSecond := s.seconds[next.Second()]
		_, okMinute := s.minutes[next.Minute()]
		_, okHour := s.hours[next.Hour()]
		_, okDay := s.dom[next.Day()]
		_, okMonth := s.months[int(next.Month())]
		_, okWeek := s.dow[int(next.Weekday())]
		if okSecond && okMinute && okHour && okDay && okMonth && okWeek {
			return next.UTC()
		}
		next = next.Add(time.Second)
	}
	return time.Time{}
}

func TestCronScheduleNextMatchesBruteForceReference(t *testing.T) {
	specs := []string{
		"0 0 9 * * *",       // 每天 9 点
		"*/15 * * * * *",    // 每 15 秒
		"0 30 8-17 * * 1-5", // 工作日每小时 30 分
		"0 0 0 1 * *",       // 每月 1 号
		"0 0 12 * * 0",      // 每周日中午
		"5/20 * * * * *",    // 裸值 + 步长
		"0 0 0 29 2 *",      // 闰年 2 月 29 日
	}
	starts := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 28, 23, 59, 50, 0, time.UTC),
		time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2028, 6, 15, 12, 0, 0, 0, time.UTC), // 2028 是闰年
	}

	for _, spec := range specs {
		sched, err := Parse(spec)
		if err != nil {
			t.Fatalf("parse %q: %v", spec, err)
		}
		cs, ok := sched.(cronSchedule)
		if !ok {
			t.Fatalf("spec %q did not produce a cronSchedule", spec)
		}
		for _, start := range starts {
			got := cs.Next(start)
			want := bruteForceNext(cs, start)
			if !got.Equal(want) {
				t.Fatalf("spec %q from %s: Next() = %s, brute-force = %s", spec, start, got, want)
			}
		}
	}
}

func TestCronScheduleNextIsFastForSparseSchedule(t *testing.T) {
	sched, err := Parse("0 0 0 1 1 *") // 每年 1 月 1 日
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	deadline := time.Now().Add(200 * time.Millisecond)

	got := sched.Next(start)
	if time.Now().After(deadline) {
		t.Fatal("Next() took too long for a once-a-year schedule; brute-force scan regressed")
	}
	want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local).UTC()
	if !got.Equal(want) {
		t.Fatalf("next run = %s, want %s", got, want)
	}
}
