package tastytrade

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestOpenInterest_ErrorUsesShortTTLAndReturnsError(t *testing.T) {
	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	val, ok, err := g.OpenInterest(context.Background(), "ERRSYM")
	if err == nil || ok || val != 0 {
		t.Fatalf("OpenInterest = (%v, %v, %v), want (0, false, err)", val, ok, err)
	}

	g.openInterestCache.mu.Lock()
	entry := g.openInterestCache.entries["ERRSYM"]
	g.openInterestCache.mu.Unlock()
	ttl := time.Until(entry.expires)
	if ttl > openInterestErrorTTL+time.Second || ttl <= 0 {
		t.Fatalf("error cache ttl = %v, want <= %v", ttl, openInterestErrorTTL)
	}
}

func TestOpenInterest_NotFoundCachedAsNoOptionsWithoutError(t *testing.T) {
	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	val, ok, err := g.OpenInterest(context.Background(), "NOCHAIN")
	if err != nil || ok || val != 0 {
		t.Fatalf("OpenInterest = (%v, %v, %v), want (0, false, nil)", val, ok, err)
	}

	g.openInterestCache.mu.Lock()
	entry := g.openInterestCache.entries["NOCHAIN"]
	g.openInterestCache.mu.Unlock()
	if time.Until(entry.expires) < 5*time.Hour {
		t.Fatalf("not found cache ttl = %v, want ~6h", time.Until(entry.expires))
	}
}
