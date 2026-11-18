package httpretry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

var fastDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

func serverWith(t *testing.T, handler func(call int, w http.ResponseWriter, r *http.Request)) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(int(calls.Add(1)), w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGetReturnsBodyOnFirstSuccess(t *testing.T) {
	srv, calls := serverWith(t, func(_ int, w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	resp, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

	if err != nil || bodyOf(t, resp) != "ok" || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want a single successful call", err, calls.Load())
	}
}

func TestGetRetriesServerErrorsUntilSuccess(t *testing.T) {
	srv, calls := serverWith(t, func(call int, w http.ResponseWriter, _ *http.Request) {
		if call < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})

	resp, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

	if err != nil || bodyOf(t, resp) != "ok" || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d, want success on the 3rd call", err, calls.Load())
	}
}

func TestGetRetriesTooManyRequests(t *testing.T) {
	srv, calls := serverWith(t, func(call int, w http.ResponseWriter, _ *http.Request) {
		if call == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	})

	_, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

	if err != nil || calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d, want a retry after 429", err, calls.Load())
	}
}

func TestGetDoesNotRetryForbiddenOrNotFound(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound} {
		srv, calls := serverWith(t, func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })

		_, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

		if StatusCode(err) != code || calls.Load() != 1 {
			t.Fatalf("code %d: status=%d calls=%d, want the status error after one call", code, StatusCode(err), calls.Load())
		}
	}
}

func TestGetReturnsLastStatusErrorWhenRetriesRunOut(t *testing.T) {
	srv, calls := serverWith(t, func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })

	_, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

	if StatusCode(err) != http.StatusBadGateway || calls.Load() != int64(len(fastDelays)+1) {
		t.Fatalf("status=%d calls=%d, want 502 after %d calls", StatusCode(err), calls.Load(), len(fastDelays)+1)
	}
}

func TestGetRetriesDroppedConnections(t *testing.T) {
	srv, calls := serverWith(t, func(call int, w http.ResponseWriter, _ *http.Request) {
		if call == 1 {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write([]byte("ok"))
	})

	resp, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

	if err != nil || bodyOf(t, resp) != "ok" || calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d, want a retry after the dropped connection", err, calls.Load())
	}
}

func TestGetAcceptsPartialContentAndSendsHeaders(t *testing.T) {
	var gotRange, gotAgent string
	srv, _ := serverWith(t, func(_ int, w http.ResponseWriter, r *http.Request) {
		gotRange, gotAgent = r.Header.Get("Range"), r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("tail"))
	})
	header := http.Header{}
	header.Set("Range", "bytes=10-")
	header.Set("User-Agent", "tester")

	resp, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Header: header})

	if err != nil || resp.StatusCode != http.StatusPartialContent || bodyOf(t, resp) != "tail" {
		t.Fatalf("err=%v, want the 206 response", err)
	}
	if gotRange != "bytes=10-" || gotAgent != "tester" {
		t.Fatalf("range=%q agent=%q, want the configured headers", gotRange, gotAgent)
	}
}

func TestGetCallsWaitBeforeEveryAttempt(t *testing.T) {
	srv, _ := serverWith(t, func(call int, w http.ResponseWriter, _ *http.Request) {
		if call < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	})
	var waits atomic.Int64

	Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays, Wait: func(context.Context) error { waits.Add(1); return nil }})

	if waits.Load() != 3 {
		t.Fatalf("waits = %d, want 3", waits.Load())
	}
}

func TestGetStopsWhenWaitFails(t *testing.T) {
	srv, calls := serverWith(t, func(_ int, w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	limiterErr := errors.New("limiter closed")

	_, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays, Wait: func(context.Context) error { return limiterErr }})

	if !errors.Is(err, limiterErr) || calls.Load() != 0 {
		t.Fatalf("err=%v calls=%d, want the limiter error and no request", err, calls.Load())
	}
}

func TestGetStopsRetryingWhenContextCancelled(t *testing.T) {
	srv, calls := serverWith(t, func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()

	_, err := Get(ctx, http.DefaultClient, srv.URL, Options{Delays: []time.Duration{time.Hour}})

	if err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want an error after the first call", err, calls.Load())
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"5":                             5 * time.Second,
		"0":                             0,
		"-3":                            0,
		"99999":                         maxRetryAfter,
		"garbage":                       0,
		"Wed, 18 Nov 2026 12:00:30 GMT": 30 * time.Second,
		"Wed, 18 Nov 2026 11:00:00 GMT": 0,
	}
	for header, want := range cases {
		if got := parseRetryAfter(header, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", header, got, want)
		}
	}
}

func TestGetHonoursRetryAfterHeader(t *testing.T) {
	srv, calls := serverWith(t, func(call int, w http.ResponseWriter, _ *http.Request) {
		if call == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	})
	start := time.Now()

	_, err := Get(context.Background(), http.DefaultClient, srv.URL, Options{Delays: fastDelays})

	if err != nil || calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d, want success on the 2nd call", err, calls.Load())
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("elapsed = %s, want the 1s Retry-After to be honoured", elapsed)
	}
}
