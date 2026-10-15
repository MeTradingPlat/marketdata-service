package tastytrade

import (
	"context"
	"sync"
	"sync/atomic"
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

func TestSessionBreaker_GuardReturnsImmediatelyWithoutCooldown(t *testing.T) {
	var b SessionBreaker
	start := time.Now()
	release, err := b.Guard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release == nil {
		t.Fatal("expected non-nil release func")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("Guard blocked for %v with no active cooldown", elapsed)
	}
	release()
}

func TestSessionBreaker_GuardSerializesConcurrentWaitersAfterActiveCooldown(t *testing.T) {
	var b SessionBreaker
	b.cooldownUntilUnixNano.Store(time.Now().Add(100 * time.Millisecond).UnixNano())

	firstAcquired := make(chan struct{})
	secondAcquired := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstReleased := make(chan struct{})

	runGuard := func() {
		release, err := b.Guard(context.Background())
		if err != nil {
			t.Errorf("Guard failed: %v", err)
			return
		}
		select {
		case <-firstAcquired:
			select {
			case <-firstReleased:
			default:
				t.Error("second Guard acquired gate before first release was called")
			}
			release()
			close(secondAcquired)
		default:
			close(firstAcquired)
			<-releaseFirst
			release()
			close(firstReleased)
		}
	}

	go runGuard()
	go runGuard()

	select {
	case <-firstAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("first Guard timed out waiting for active cooldown")
	}

	select {
	case <-secondAcquired:
		t.Fatal("second Guard acquired gate before first release was called")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)

	select {
	case <-secondAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("second Guard timed out after first release was called")
	}
}

func TestSessionBreaker_GuardReblocksIfSaturatedAgainBeforeRelease(t *testing.T) {
	var b SessionBreaker
	b.cooldownUntilUnixNano.Store(time.Now().Add(500 * time.Millisecond).UnixNano())

	var holderClaimed atomic.Bool
	holderAcquired := make(chan struct{})
	releaseHolder := make(chan struct{})
	waiterAcquired := make(chan struct{})

	runGuard := func() {
		release, err := b.Guard(context.Background())
		if err != nil {
			t.Errorf("Guard failed: %v", err)
			return
		}
		if holderClaimed.CompareAndSwap(false, true) {
			close(holderAcquired)
			<-releaseHolder
			b.MarkSaturated()
			release()
			return
		}
		release()
		close(waiterAcquired)
	}

	go runGuard()
	go runGuard()

	select {
	case <-holderAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("no Guard call acquired the gate in time")
	}

	select {
	case <-waiterAcquired:
		t.Fatal("second Guard acquired the gate before the holder released it")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseHolder)

	select {
	case <-waiterAcquired:
		t.Fatal("waiter proceeded immediately after a fresh MarkSaturated instead of re-blocking for the new window")
	case <-time.After(200 * time.Millisecond):
	}
}
