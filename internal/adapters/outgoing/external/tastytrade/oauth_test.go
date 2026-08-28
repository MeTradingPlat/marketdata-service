package tastytrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// Regression: cada DxLinkConn del pool llama ResetSessions por su cuenta
// cuando detecta su propia sesion saturada -- sin coalescer, decenas de
// llamadas casi simultaneas (tipico en la apertura del mercado) mandan cada
// una su propio DELETE /sessions, y cada uno invalida la sesion recien
// creada por el anterior. Confirmado en vivo el 2026-08-28: silencio total
// de DxLink por horas en pleno mercado abierto, sin recuperarse solo.
func TestResetSessions_ConcurrentCallsCollapseIntoOne(t *testing.T) {
	var deletes, refreshes int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/sessions":
			atomic.AddInt32(&deletes, 1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			atomic.AddInt32(&refreshes, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-token","refresh_token":"fresh-refresh"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oauth := NewOAuth(OAuthConfig{BaseURL: server.URL, RefreshToken: "initial-refresh"})
	// LogoutAllSessions es un no-op sin token todavia -- establecer uno
	// primero (como ya tendria cualquier conexion que llegue a detectar
	// session-saturada de verdad) para que el DELETE concurrente de abajo
	// tenga algo real que colapsar.
	if _, err := oauth.RefreshAccessToken(context.Background()); err != nil {
		t.Fatalf("setup: unexpected error priming the access token: %v", err)
	}
	atomic.StoreInt32(&refreshes, 0)

	const callers = 30
	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = oauth.ResetSessions(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&deletes); got != 1 {
		t.Errorf("expected exactly 1 DELETE /sessions call from %d concurrent ResetSessions, got %d", callers, got)
	}
	if got := atomic.LoadInt32(&refreshes); got != 1 {
		t.Errorf("expected exactly 1 token refresh call from %d concurrent ResetSessions, got %d", callers, got)
	}
}
