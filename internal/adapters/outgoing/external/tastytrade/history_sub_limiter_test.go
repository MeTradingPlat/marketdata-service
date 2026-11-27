package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestHistorySubLimiter_WaitsOnQuotaAndResumesOnExpiry(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	lim := newHistorySubLimiterWithClock(3, 60*time.Second, func() time.Time { return now })
	res, err := lim.acquire(context.Background(), 1)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	res.commitAdds(1)
	res.release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := lim.acquire(ctx, 1); err == nil {
		t.Fatal("expected second acquire to wait and time out")
	}

	now = now.Add(60 * time.Second)
	res2, err := lim.acquire(context.Background(), 1)
	if err != nil {
		t.Fatalf("acquire after expiry failed: %v", err)
	}
	res2.release()
}

func TestHistorySubLimiter_RemovesNeverWaitButCount(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	lim := newHistorySubLimiterWithClock(4, 60*time.Second, func() time.Time { return now })
	lim.recordRemoves(4)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := lim.acquire(ctx, 1); err == nil {
		t.Fatal("expected acquire to wait after removes filled quota")
	}
}
