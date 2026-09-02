package tastytrade

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSessionBreaker_MarkSaturatedDoesNotExtendActiveWindow(t *testing.T) {
	var b SessionBreaker
	b.MarkSaturated()
	first := b.cooldownUntilUnixNano.Load()

	time.Sleep(10 * time.Millisecond)
	b.MarkSaturated()
	second := b.cooldownUntilUnixNano.Load()

	if first != second {
		t.Errorf("MarkSaturated extended an already-active window: first=%d second=%d", first, second)
	}
}

func TestSessionBreaker_MarkSaturatedStartsANewWindowAfterExpiry(t *testing.T) {
	var b SessionBreaker
	b.cooldownUntilUnixNano.Store(time.Now().Add(-time.Second).UnixNano())

	b.MarkSaturated()
	got := b.cooldownUntilUnixNano.Load()

	if got <= time.Now().UnixNano() {
		t.Errorf("MarkSaturated did not open a new window once the previous one expired")
	}
}

func TestSessionBreaker_MarkSaturatedConcurrentCallsAgreeOnOneDeadline(t *testing.T) {
	var b SessionBreaker
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.MarkSaturated()
		}()
	}
	wg.Wait()

	deadline := b.cooldownUntilUnixNano.Load()
	if deadline <= time.Now().UnixNano() {
		t.Fatal("expected an active cooldown window after concurrent MarkSaturated calls")
	}
}

func TestSessionBreaker_WaitReturnsImmediatelyWithoutSaturation(t *testing.T) {
	var b SessionBreaker
	start := time.Now()
	if err := b.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("Wait blocked for %v with no active cooldown", elapsed)
	}
}

func TestSessionBreaker_WaitBlocksUntilCooldownEnds(t *testing.T) {
	var b SessionBreaker
	b.cooldownUntilUnixNano.Store(time.Now().Add(30 * time.Millisecond).UnixNano())

	start := time.Now()
	if err := b.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Errorf("Wait returned before the cooldown ended: %v", elapsed)
	}
}

func TestSessionBreaker_WaitRespectsContextCancellation(t *testing.T) {
	var b SessionBreaker
	b.cooldownUntilUnixNano.Store(time.Now().Add(time.Hour).UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := b.Wait(ctx); err == nil {
		t.Error("expected Wait to return the context error instead of blocking for the full cooldown")
	}
}
