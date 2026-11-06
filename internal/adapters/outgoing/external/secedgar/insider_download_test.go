package secedgar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadInsiderQuarterFallsBackToLegacy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/legacy/2026q1.zip" {
			w.Write([]byte("zipdata"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	templates := []string{srv.URL + "/new/%dq%d.zip", srv.URL + "/legacy/%dq%d.zip"}
	target := filepath.Join(t.TempDir(), "insider.zip")

	if err := downloadInsiderQuarter(context.Background(), templates, 2026, 1, target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading target: %v", err)
	}
	if string(got) != "zipdata" {
		t.Fatalf("content = %q, want %q", got, "zipdata")
	}
}

func TestDownloadInsiderQuarterUnavailableOnBothPaths(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	templates := []string{srv.URL + "/new/%dq%d.zip", srv.URL + "/legacy/%dq%d.zip"}
	target := filepath.Join(t.TempDir(), "insider.zip")

	if err := downloadInsiderQuarter(context.Background(), templates, 2026, 1, target); err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target should not exist, stat err = %v", err)
	}
}
