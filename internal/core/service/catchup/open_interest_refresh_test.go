package catchup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	fundamentalscache "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

type fakeOpenInterestGateway struct {
	results map[string]struct {
		val float64
		ok  bool
		err error
	}
}

func (f *fakeOpenInterestGateway) OpenInterest(ctx context.Context, symbol string) (float64, bool, error) {
	r := f.results[symbol]
	return r.val, r.ok, r.err
}

type fakeOIFundamentals struct {
	out.FundamentalsRepository
	stale     []string
	found     map[string]float64
	attempted []string
}

func (f *fakeOIFundamentals) GetSymbolsWithStaleOpenInterest(ctx context.Context, windowStart time.Time) ([]string, error) {
	return f.stale, nil
}

func (f *fakeOIFundamentals) UpsertOpenInterestBatch(ctx context.Context, openInterests map[string]float64, attemptedOnly []string) error {
	f.found = openInterests
	f.attempted = attemptedOnly
	return nil
}

func TestRefreshOpenInterest_SkipsAttemptedOnError(t *testing.T) {
	repo := &fakeOIFundamentals{stale: []string{"AAPL", "NOOPT", "ERRSYM"}}
	gw := &fakeOpenInterestGateway{results: map[string]struct {
		val float64
		ok  bool
		err error
	}{
		"AAPL":   {val: 500, ok: true},
		"NOOPT":  {val: 0, ok: false},
		"ERRSYM": {err: errors.New("500")},
	}}
	cache := fundamentalscache.NewFundamentalsCache(nil, nil, nil)
	if err := RefreshOpenInterest(context.Background(), gw, repo, cache, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.found["AAPL"] != 500 || len(repo.attempted) != 1 || repo.attempted[0] != "NOOPT" {
		t.Fatalf("found=%v attempted=%v, want AAPL=500 and [NOOPT]", repo.found, repo.attempted)
	}
}
