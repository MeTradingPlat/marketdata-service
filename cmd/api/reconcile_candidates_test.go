package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func symbolsNamed(names ...string) []domain.Symbol {
	out := make([]domain.Symbol, len(names))
	for i, n := range names {
		out[i] = domain.Symbol{Symbol: n}
	}
	return out
}

func probesFrom(attempted, live, subscribed map[string]bool) reconcileProbes {
	return reconcileProbes{
		isAttempted: func(s string) bool { return attempted[s] },
		isLive:      func(s string) bool { return live[s] },
		subscribed:  func(s string) bool { return subscribed[s] },
	}
}

func TestCandidatesSkipHealthyAndIncludeDropped(t *testing.T) {
	attempted := map[string]bool{"OK": true, "DROPPED": true}
	live := map[string]bool{"OK": true, "DROPPED": true}
	subscribed := map[string]bool{"OK": true}

	got := selectReconcileCandidates(symbolsNamed("OK", "DROPPED"), probesFrom(attempted, live, subscribed), true, newReconcileBackoff(), time.Now(), 10)

	if !reflect.DeepEqual(got, []string{"DROPPED"}) {
		t.Fatalf("candidates = %v, want [DROPPED]", got)
	}
}

func TestCandidatesLeaveNeverAttemptedAloneWhileRolloutRuns(t *testing.T) {
	probes := probesFrom(nil, nil, nil)

	during := selectReconcileCandidates(symbolsNamed("NEW"), probes, false, newReconcileBackoff(), time.Now(), 10)
	after := selectReconcileCandidates(symbolsNamed("NEW"), probes, true, newReconcileBackoff(), time.Now(), 10)

	if len(during) != 0 || len(after) != 1 {
		t.Fatalf("during=%v after=%v, want none during and one after the rollout", during, after)
	}
}

func TestCandidatesSkipSymbolsInBackoff(t *testing.T) {
	attempted := map[string]bool{"A": true, "B": true}
	backoff := newFixedBackoff()
	now := time.Now()
	backoff.recordFailure("A", now)

	got := selectReconcileCandidates(symbolsNamed("A", "B"), probesFrom(attempted, nil, nil), true, backoff, now, 10)

	if !reflect.DeepEqual(got, []string{"B"}) {
		t.Fatalf("candidates = %v, want [B]", got)
	}
}

func TestCandidatesRespectLimit(t *testing.T) {
	names := make([]string, 50)
	attempted := make(map[string]bool, 50)
	for i := range names {
		names[i] = fmt.Sprintf("S%d", i)
		attempted[names[i]] = true
	}

	got := selectReconcileCandidates(symbolsNamed(names...), probesFrom(attempted, nil, nil), true, newReconcileBackoff(), time.Now(), 7)

	if len(got) != 7 {
		t.Fatalf("len(candidates) = %d, want 7", len(got))
	}
}
