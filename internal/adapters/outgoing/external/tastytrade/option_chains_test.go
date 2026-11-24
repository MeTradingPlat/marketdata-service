package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func newOpenInterestTestGateway(t *testing.T, handler http.HandlerFunc) *Gateway {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oauth := NewOAuth(OAuthConfig{BaseURL: srv.URL, RefreshToken: "refresh"})
	oauth.accessToken = "test-token"
	return NewGateway(oauth, nil)
}

func futureDate(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01-02")
}

func TestOpenInterest_PrefersRegularMonthlyAndSendsOnlyMonthlySymbols(t *testing.T) {
	weeklyDate := futureDate(3)
	monthlyDate := futureDate(20)

	var gotByType [][]string
	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/option-chains/SPY/nested":
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{
							"option-chain-type": "Standard",
							"expirations": []map[string]any{
								{
									"expiration-date": weeklyDate,
									"expiration-type": "Weekly",
									"strikes":         []map[string]any{{"call": "WEEKLY_C", "put": "WEEKLY_P"}},
								},
								{
									"expiration-date": monthlyDate,
									"expiration-type": "Regular",
									"strikes":         []map[string]any{{"call": "MONTHLY_C", "put": "MONTHLY_P"}},
								},
							},
						},
						{
							"option-chain-type": "NonStandard",
							"expirations": []map[string]any{
								{
									"expiration-date": futureDate(10),
									"expiration-type": "Regular",
									"strikes":         []map[string]any{{"call": "NONSTD_C", "put": "NONSTD_P"}},
								},
							},
						},
					},
				},
			})
		case "/market-data/by-type":
			gotByType = append(gotByType, r.URL.Query()["equity-option"])
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"symbol": "MONTHLY_C", "open-interest": "120"},
						{"symbol": "MONTHLY_P", "open-interest": 80},
					},
				},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	val, ok := g.OpenInterest(context.Background(), "SPY")
	if !ok || val != 200 {
		t.Fatalf("OpenInterest = (%v, %v), want (200, true)", val, ok)
	}
	wantQueries := [][]string{{"MONTHLY_C", "MONTHLY_P"}}
	if !reflect.DeepEqual(gotByType, wantQueries) {
		t.Fatalf("equity-option queries = %v, want %v", gotByType, wantQueries)
	}
}

func TestOpenInterest_CacheHitsAvoidDuplicateRequests(t *testing.T) {
	monthlyDate := futureDate(20)
	var chainCalls, quoteCalls int

	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/option-chains/AAPL/nested":
			chainCalls++
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{
							"option-chain-type": "Standard",
							"expirations": []map[string]any{
								{
									"expiration-date": monthlyDate,
									"expiration-type": "Regular",
									"strikes":         []map[string]any{{"call": "AAPL_C", "put": "AAPL_P"}},
								},
							},
						},
					},
				},
			})
		case "/market-data/by-type":
			quoteCalls++
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"symbol": "AAPL_C", "open-interest": "50"},
						{"symbol": "AAPL_P", "open-interest": "25"},
					},
				},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	val1, ok1 := g.OpenInterest(context.Background(), "AAPL")
	val2, ok2 := g.OpenInterest(context.Background(), "AAPL")

	if !ok1 || val1 != 75 || !ok2 || val2 != 75 {
		t.Fatalf("got (%v,%v) and (%v,%v), want (75,true)", val1, ok1, val2, ok2)
	}
	if chainCalls != 1 || quoteCalls != 1 {
		t.Fatalf("chainCalls=%d quoteCalls=%d, want 1 and 1", chainCalls, quoteCalls)
	}
}

func TestOpenInterest_EmptyChainReturnsFalseWithoutCallingByType(t *testing.T) {
	var quoteCalls int
	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/option-chains/NOOPT/nested":
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"items": []any{}},
			})
		case "/market-data/by-type":
			quoteCalls++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	val, ok := g.OpenInterest(context.Background(), "NOOPT")
	if ok || val != 0 {
		t.Fatalf("OpenInterest = (%v, %v), want (0, false)", val, ok)
	}
	if quoteCalls != 0 {
		t.Fatalf("quoteCalls = %d, want 0", quoteCalls)
	}
}

func TestOpenInterest_SplitsMoreThan100ContractsIntoChunks(t *testing.T) {
	monthlyDate := futureDate(20)
	strikes := make([]map[string]any, 120)
	for i := 0; i < 120; i++ {
		strikes[i] = map[string]any{
			"call": fmt.Sprintf("OCC_C_%03d", i),
			"put":  fmt.Sprintf("OCC_P_%03d", i),
		}
	}

	var mu sync.Mutex
	var chunkSizes []int

	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/option-chains/QQQ/nested":
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{
							"option-chain-type": "Standard",
							"expirations": []map[string]any{
								{
									"expiration-date": monthlyDate,
									"expiration-type": "Regular",
									"strikes":         strikes,
								},
							},
						},
					},
				},
			})
		case "/market-data/by-type":
			syms := r.URL.Query()["equity-option"]
			mu.Lock()
			chunkSizes = append(chunkSizes, len(syms))
			mu.Unlock()

			items := make([]map[string]any, len(syms))
			for i, s := range syms {
				items[i] = map[string]any{"symbol": s, "open-interest": "10"}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"items": items},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	val, ok := g.OpenInterest(context.Background(), "QQQ")
	if !ok || val != 2400 {
		t.Fatalf("OpenInterest = (%v, %v), want (2400, true)", val, ok)
	}
	wantSizes := []int{100, 100, 40}
	if !reflect.DeepEqual(chunkSizes, wantSizes) {
		t.Fatalf("chunkSizes = %v, want %v", chunkSizes, wantSizes)
	}
}

func TestOpenInterest_WeekliesOnlyFallsBackToNearestFuture(t *testing.T) {
	nearWeekly := futureDate(4)
	farWeekly := futureDate(11)

	var gotSymbols []string
	g := newOpenInterestTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/option-chains/WEEK/nested":
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{
							"option-chain-type": "Standard",
							"expirations": []map[string]any{
								{
									"expiration-date": farWeekly,
									"expiration-type": "Weekly",
									"strikes":         []map[string]any{{"call": "FAR_C", "put": "FAR_P"}},
								},
								{
									"expiration-date": nearWeekly,
									"expiration-type": "Weekly",
									"strikes":         []map[string]any{{"call": "NEAR_C", "put": "NEAR_P"}},
								},
							},
						},
					},
				},
			})
		case "/market-data/by-type":
			gotSymbols = r.URL.Query()["equity-option"]
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{"symbol": "NEAR_C", "open-interest": "30"},
						{"symbol": "NEAR_P"},
					},
				},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	val, ok := g.OpenInterest(context.Background(), "WEEK")
	if !ok || val != 30 {
		t.Fatalf("OpenInterest = (%v, %v), want (30, true)", val, ok)
	}
	if !reflect.DeepEqual(gotSymbols, []string{"NEAR_C", "NEAR_P"}) {
		t.Fatalf("equity-option symbols = %v, want [NEAR_C NEAR_P]", gotSymbols)
	}
}
