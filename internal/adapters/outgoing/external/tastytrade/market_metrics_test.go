package tastytrade

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestFetchMarketMetricsChunk(t *testing.T) {
	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/market-metrics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbols") == "FAIL" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"symbol": "NOEPS", "market-cap": 100.0, "beta": "1.1", "liquidity-rating": 4, "lendability": "Easy", "earnings": map[string]any{"expected-report-date": "2026-12-01"}},
					{"symbol": "ZEROEPS", "earnings-per-share": "0"},
					{"symbol": "BADEPS", "earnings-per-share": "N/A"},
				},
			},
		})
	})

	got, err := g.fetchMarketMetricsChunk(context.Background(), []string{"NOEPS", "ZEROEPS", "BADEPS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 || got[0].Eps != nil || got[0].MarketCap != 100 || got[0].NextEarningsDate != "2026-12-01" {
		t.Fatalf("NOEPS mismatch: %+v", got[0])
	}
	if got[1].Eps == nil || *got[1].Eps != 0 {
		t.Fatalf("ZEROEPS Eps = %v, want 0", got[1].Eps)
	}
	if got[2].Eps != nil {
		t.Fatalf("BADEPS Eps = %v, want nil", *got[2].Eps)
	}
	if _, err := g.fetchMarketMetricsChunk(context.Background(), []string{"FAIL"}); err == nil {
		t.Fatal("expected error on 500")
	}
}
