package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestHistorySubLimiter_NilAndDoubleRelease(t *testing.T) {
	var nilLim *historySubLimiter
	res, err := nilLim.acquire(context.Background(), 5)
	if err != nil {
		t.Fatalf("nil limiter acquire error = %v", err)
	}
	res.commitAdds(2)
	res.release()
	res.release()
	nilLim.recordRemoves(2)

	lim := newHistorySubLimiterWithClock(6, 60*time.Second, time.Now)
	r1, _ := lim.acquire(context.Background(), 1)
	r2, _ := lim.acquire(context.Background(), 1)
	r1.release()
	r1.release()
	lim.mu.Lock()
	gotReserved := lim.reserved
	lim.mu.Unlock()
	if gotReserved != 3 {
		t.Fatalf("reserved after double release = %d, want 3", gotReserved)
	}
	r2.release()
}

func TestHistorySubLimiter_PartialCommitReleasesRemainder(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	lim := newHistorySubLimiterWithClock(6, 60*time.Second, func() time.Time { return now })
	res, err := lim.acquire(context.Background(), 2)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	res.commitAdds(1)
	res.release()

	lim.mu.Lock()
	gotUsed, gotReserved := lim.used, lim.reserved
	lim.mu.Unlock()
	if gotUsed != 1 || gotReserved != 0 {
		t.Fatalf("got (used=%d, reserved=%d), want (1, 0)", gotUsed, gotReserved)
	}
}
