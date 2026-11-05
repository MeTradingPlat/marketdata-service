package tastytrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Regression: cada DxLinkConn del pool llama ResetSessions por su cuenta
// cuando detecta su propia sesion saturada -- sin coalescer, decenas de
// llamadas casi simultaneas (tipico en la apertura del mercado) mandan cada
// una su propio DELETE /sessions, y cada uno invalida la sesion recien
// creada por el anterior. Confirmado en vivo el 2026-08-28: silencio total
// de DxLink por horas en pleno mercado abierto, sin recuperarse solo.
func TestResetSessions_ConcurrentCallsCollapseIntoOne(t *testing.T) {
	const callers = 30
	var deletes, refreshes int32
	var started int32
	var gate atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/sessions":
			atomic.AddInt32(&deletes, 1)
			w.WriteHeader(http.StatusForbidden)
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			atomic.AddInt32(&refreshes, 1)
			if gate.Load() {
				for atomic.LoadInt32(&started) < callers {
					time.Sleep(time.Millisecond)
				}
				time.Sleep(50 * time.Millisecond)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-token","refresh_token":"fresh-refresh"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oauth := NewOAuth(OAuthConfig{BaseURL: server.URL, RefreshToken: "initial-refresh"})
	if _, err := oauth.RefreshAccessToken(context.Background()); err != nil {
		t.Fatalf("setup: unexpected error priming the access token: %v", err)
	}
	atomic.StoreInt32(&refreshes, 0)
	gate.Store(true)

	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			atomic.AddInt32(&started, 1)
			errs[i] = oauth.ResetSessions(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&deletes); got != 0 {
		t.Errorf("expected no DELETE /sessions call (dead endpoint) from %d concurrent ResetSessions, got %d", callers, got)
	}
	if got := atomic.LoadInt32(&refreshes); got != 1 {
		t.Errorf("expected exactly 1 token refresh call from %d concurrent ResetSessions, got %d", callers, got)
	}
}
