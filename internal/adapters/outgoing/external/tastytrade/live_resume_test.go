package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_Resume_SecondDropBeforeSettleKeepsOldest(t *testing.T) {
	p := newTestPool()
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	older := t0.Add(-10 * time.Minute)
	newer := t0.Add(-2 * time.Minute)

	p.holdResume("AAPL", older, t0)
	p.noteResumeData("AAPL", t0)
	p.noteResumeData("AAPL", t0.Add(time.Minute))
	p.holdResume("AAPL", newer, t0.Add(time.Minute))

	got := p.resolveResume("AAPL", time.Time{}, t0.Add(time.Minute))
	if !got.Equal(older) {
		t.Fatalf("expected %v, got %v", older, got)
	}
}

func TestCandlePool_Resume_ForgetsAfterSettleWithData(t *testing.T) {
	p := newTestPool()
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	older := t0.Add(-20 * time.Minute)

	p.holdResume("AAPL", older, t0)
	p.noteResumeData("AAPL", t0)
	p.noteResumeData("AAPL", t0.Add(resumeSettle+time.Second))

	now := t0.Add(3 * time.Minute)
	got := p.resolveResume("AAPL", time.Time{}, now)
	want := now.Add(-5 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("expected fallback %v after settle, got %v", want, got)
	}
}

func TestCandlePool_Resume_DropBeforeSettleResetsFirstDataAt(t *testing.T) {
	p := newTestPool()
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	older := t0.Add(-15 * time.Minute)
	newer := t0.Add(-2 * time.Minute)

	p.holdResume("AAPL", older, t0)
	p.noteResumeData("AAPL", t0)
	p.holdResume("AAPL", newer, t0.Add(time.Minute))
	p.noteResumeData("AAPL", t0.Add(3*time.Minute))

	got := p.resolveResume("AAPL", time.Time{}, t0.Add(3*time.Minute))
	if !got.Equal(older) {
		t.Fatalf("expected %v, got %v", older, got)
	}
}

func TestCandlePool_Resume_SettledEntryIgnoredWithoutMoreEvents(t *testing.T) {
	p := newTestPool()
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	older := t0.Add(-20 * time.Minute)
	newer := t0.Add(-2 * time.Minute)

	p.holdResume("AAPL", older, t0)
	p.noteResumeData("AAPL", t0)

	later := t0.Add(3 * time.Minute)
	got := p.resolveResume("AAPL", time.Time{}, later)
	if !got.Equal(older) {
		t.Fatalf("expected %v kept without more events, got %v", older, got)
	}

	p.holdResume("AAPL", newer, later)
	got = p.resolveResume("AAPL", time.Time{}, later)
	if !got.Equal(older) {
		t.Fatalf("expected %v kept after newer hold, got %v", older, got)
	}
}

func TestCandlePool_Resume_DropAfterDataAndLongPauseClearsOnlyOnSecondData(t *testing.T) {
	p := newTestPool()
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	older := t0.Add(-20 * time.Minute)
	newer := t0.Add(-2 * time.Minute)

	p.holdResume("AAPL", older, t0)
	p.noteResumeData("AAPL", t0)

	later := t0.Add(10 * time.Minute)
	p.holdResume("AAPL", newer, later)
	if got := p.resolveResume("AAPL", time.Time{}, later); !got.Equal(older) {
		t.Fatalf("expected %v kept after 10m drop, got %v", older, got)
	}

	p.noteResumeData("AAPL", later)
	p.noteResumeData("AAPL", later.Add(resumeSettle+time.Second))

	after := later.Add(3 * time.Minute)
	want := after.Add(-5 * time.Minute)
	if got := p.resolveResume("AAPL", time.Time{}, after); !got.Equal(want) {
		t.Fatalf("expected fallback %v after second data post-settle, got %v", want, got)
	}
}

func TestCandlePool_Resume_FormingCandleFallbackWithoutReconnect(t *testing.T) {
	p := newTestPool()
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	formingTS := t0.Add(-7 * time.Minute)

	p.currentMu.Lock()
	p.current["AAPL"] = domain.Candle{Symbol: "AAPL", Timestamp: formingTS}
	p.currentMu.Unlock()

	got := p.resolveResume("AAPL", time.Time{}, t0)
	if !got.Equal(formingTS) {
		t.Fatalf("expected forming timestamp %v, got %v", formingTS, got)
	}
}
