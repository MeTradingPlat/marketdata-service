package tastytrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

type tokenServer struct {
	srv           *httptest.Server
	refreshCalls  atomic.Int64
	validToken    string
	issuedToken   string
	refreshStatus int
}

func newTokenServer(validToken, issuedToken string, refreshStatus int) *tokenServer {
	ts := &tokenServer{validToken: validToken, issuedToken: issuedToken, refreshStatus: refreshStatus}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			ts.refreshCalls.Add(1)
			w.WriteHeader(ts.refreshStatus)
			w.Write([]byte(`{"access_token":"` + ts.issuedToken + `"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+ts.validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ok":"yes"}`))
	}))
	return ts
}

func gatewayWithToken(ts *tokenServer, token string) *Gateway {
	oauth := NewOAuth(OAuthConfig{BaseURL: ts.srv.URL, RefreshToken: "refresh"})
	oauth.accessToken = token
	return &Gateway{oauth: oauth}
}

func TestDoAuthenticatedJSONRefreshesAndRetriesOnUnauthorized(t *testing.T) {
	ts := newTokenServer("new", "new", http.StatusOK)
	defer ts.srv.Close()
	g := gatewayWithToken(ts, "old")

	out, err := doAuthenticatedJSON[map[string]string](context.Background(), g, ts.srv.URL+"/data")

	if err != nil || out["ok"] != "yes" {
		t.Fatalf("out=%v err=%v, want ok=yes", out, err)
	}
	if got := ts.refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func TestDoAuthenticatedJSONRetriesOnlyOnce(t *testing.T) {
	ts := newTokenServer("only-this-works", "issued-but-rejected", http.StatusOK)
	defer ts.srv.Close()
	g := gatewayWithToken(ts, "old")

	_, err := doAuthenticatedJSON[map[string]string](context.Background(), g, ts.srv.URL+"/data")

	if err == nil {
		t.Fatal("expected an error when the retry is also unauthorized")
	}
	if got := ts.refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func TestDoAuthenticatedJSONReturnsUnauthorizedWhenRefreshFails(t *testing.T) {
	ts := newTokenServer("new", "new", http.StatusInternalServerError)
	defer ts.srv.Close()
	g := gatewayWithToken(ts, "old")

	_, err := doAuthenticatedJSON[map[string]string](context.Background(), g, ts.srv.URL+"/data")

	if err == nil {
		t.Fatal("expected the original 401 error")
	}
}

func TestConcurrentUnauthorizedRequestsShareOneRefresh(t *testing.T) {
	ts := newTokenServer("new", "new", http.StatusOK)
	defer ts.srv.Close()
	g := gatewayWithToken(ts, "old")

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := doAuthenticatedJSON[map[string]string](context.Background(), g, ts.srv.URL+"/data"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := ts.refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}
