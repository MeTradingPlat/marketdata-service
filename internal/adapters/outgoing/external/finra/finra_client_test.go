package finra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const finraSampleCsv = "accountingYearMonthNumber|symbolCode|issueName|issuerServicesGroupExchangeCode|marketClassCode|currentShortPositionQuantity|previousShortPositionQuantity|stockSplitFlag|averageDailyVolumeQuantity|daysToCoverQuantity|revisionFlag|changePercent|changePreviousNumber|settlementDate\n20261113|AAPL|Apple|NYSE|NNM|1000|900||500|2.0||11.1|100|2026-11-13\n"

func clientFor(srv *httptest.Server, dates ...string) *Client {
	candidates := make([]time.Time, len(dates))
	for i, d := range dates {
		candidates[i], _ = time.Parse("20060102", d)
	}
	return &Client{
		urlTemplate: srv.URL + "/shrt%s.csv",
		delays:      []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		candidates:  func(int) []time.Time { return candidates },
	}
}

type fakeCdn struct {
	mu       sync.Mutex
	statuses map[string][]int
	hits     map[string]int
}

func newFakeCdn(statuses map[string][]int) (*fakeCdn, *httptest.Server) {
	cdn := &fakeCdn{statuses: statuses, hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/shrt"), ".csv")
		cdn.mu.Lock()
		call := cdn.hits[date]
		cdn.hits[date]++
		script := cdn.statuses[date]
		status := script[min(call, len(script)-1)]
		cdn.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Write([]byte(finraSampleCsv))
	}))
	return cdn, srv
}

func TestDownloadLatestFallsBackWhenNewestIsNotPublished(t *testing.T) {
	for _, missing := range []int{http.StatusForbidden, http.StatusNotFound} {
		cdn, srv := newFakeCdn(map[string][]int{"20261130": {missing}, "20261113": {http.StatusOK}})
		defer srv.Close()

		result, err := clientFor(srv, "20261130", "20261113").DownloadLatest(context.Background())

		if err != nil || len(result) != 1 {
			t.Fatalf("status %d: err=%v records=%d, want the older file", missing, err, len(result))
		}
		if cdn.hits["20261130"] != 1 {
			t.Fatalf("status %d: hits=%d, a missing file must not be retried", missing, cdn.hits["20261130"])
		}
	}
}

func TestDownloadLatestRetriesTransientErrorsOnTheSameFile(t *testing.T) {
	cdn, srv := newFakeCdn(map[string][]int{"20261130": {http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusOK}, "20261113": {http.StatusOK}})
	defer srv.Close()

	result, err := clientFor(srv, "20261130", "20261113").DownloadLatest(context.Background())

	if err != nil || len(result) != 1 {
		t.Fatalf("err=%v records=%d, want the newest file after retries", err, len(result))
	}
	if cdn.hits["20261113"] != 0 {
		t.Fatal("must not fall back to the older file while the newest one is recoverable")
	}
}

func TestDownloadLatestFailsInsteadOfUsingStaleFileOnPersistentServerError(t *testing.T) {
	cdn, srv := newFakeCdn(map[string][]int{"20261130": {http.StatusInternalServerError}, "20261113": {http.StatusOK}})
	defer srv.Close()

	_, err := clientFor(srv, "20261130", "20261113").DownloadLatest(context.Background())

	if err == nil {
		t.Fatal("expected an error instead of silently using the older file")
	}
	if cdn.hits["20261113"] != 0 {
		t.Fatal("the older file must not be downloaded after a persistent server error")
	}
}

func TestDownloadLatestFailsWhenNothingIsPublished(t *testing.T) {
	_, srv := newFakeCdn(map[string][]int{"20261130": {http.StatusForbidden}, "20261113": {http.StatusForbidden}})
	defer srv.Close()

	_, err := clientFor(srv, "20261130", "20261113").DownloadLatest(context.Background())

	if err == nil {
		t.Fatal("expected an error when no candidate is published")
	}
}
