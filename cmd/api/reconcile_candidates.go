package main

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type reconcileProbes struct {
	isAttempted func(string) bool
	isLive      func(string) bool
	subscribed  func(string) bool
}

func selectReconcileCandidates(tracked []domain.Symbol, probes reconcileProbes, rolloutDone bool, backoff *reconcileBackoff, now time.Time, limit int) []string {
	candidates := make([]string, 0, limit)
	for _, s := range tracked {
		if len(candidates) >= limit {
			break
		}
		if shouldSkipReconcileRetry(probes.isAttempted(s.Symbol), rolloutDone, probes.isLive(s.Symbol), probes.subscribed(s.Symbol)) {
			continue
		}
		if !backoff.due(s.Symbol, now) {
			continue
		}
		candidates = append(candidates, s.Symbol)
	}
	return candidates
}
