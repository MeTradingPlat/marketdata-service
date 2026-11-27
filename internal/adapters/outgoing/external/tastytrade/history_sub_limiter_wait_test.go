package tastytrade

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestHistorySubLimiter_WaitBeforeDownloadTimeout(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	lim := newHistorySubLimiterWithClock(3, 50*time.Millisecond, func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	})
	res, _ := lim.acquire(context.Background(), 1)
	res.commitAdds(1)
	res.release()
	go func() {
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		now = now.Add(60 * time.Millisecond)
		mu.Unlock()
	}()
	start := time.Now()
	res2, err := lim.acquire(context.Background(), 1)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	res2.release()
	if time.Since(start) < 10*time.Millisecond {
		t.Fatal("expected acquire to wait before returning")
	}
	p := NewCandlePool(nil, 1)
	p.SetHistorySubChangesPerMinute(0)
	p.SetHistorySubChangesPerMinute(1234)
	(*historySubLimiter)(nil).setQuota(10)
}
