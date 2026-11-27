package tastytrade

import (
	"sync"
	"testing"
	"time"
)

func TestSubChangeCounter_LogsAtMostOncePerMinute(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	type logEntry struct {
		changes, adds, removes, windowSeconds int
	}
	var logs []logEntry
	c := newSubChangeCounter(time.Minute, func() time.Time { return now }, func(ch, a, r, w int) {
		logs = append(logs, logEntry{ch, a, r, w})
	})

	c.record(2, 1)
	now = now.Add(30 * time.Second)
	c.record(1, 2)
	if len(logs) != 0 {
		t.Fatalf("expected 0 logs before 1 minute, got %d", len(logs))
	}

	now = now.Add(30 * time.Second)
	c.record(3, 0)
	if len(logs) != 1 || logs[0] != (logEntry{9, 6, 3, 60}) {
		t.Fatalf("logs = %+v, want [{9 6 3 60}]", logs)
	}

	now = now.Add(30 * time.Second)
	c.record(1, 0)
	if len(logs) != 1 {
		t.Fatalf("expected still 1 log, got %d", len(logs))
	}
}

func TestSubChangeCounter_Concurrent(t *testing.T) {
	c := newSubChangeCounter(time.Hour, time.Now, func(int, int, int, int) {})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.record(1, 2)
		}()
	}
	wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.adds != 10 || c.removes != 20 {
		t.Fatalf("adds=%d removes=%d, want 10 and 20", c.adds, c.removes)
	}
}
