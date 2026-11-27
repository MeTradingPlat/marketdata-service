package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestHistorySubLimiter_WindowStartsAtCommitNotAcquire(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	lim := newHistorySubLimiterWithClock(3, 60*time.Second, func() time.Time { return now })

	res, err := lim.acquire(context.Background(), 1)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}

	now = now.Add(65 * time.Second)
	res.commitAdds(1)
	res.release()

	now = now.Add(59 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := lim.acquire(ctx, 1); err == nil {
		t.Fatal("expected second acquire to wait 60s after commit, not acquire")
	}

	now = now.Add(time.Second)
	res2, err := lim.acquire(context.Background(), 1)
	if err != nil {
		t.Fatalf("acquire 60s after commit failed: %v", err)
	}
	res2.release()
}
