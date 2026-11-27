package tastytrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDxLinkConn_CloseDuringCooldown_AbortsWithoutDial(t *testing.T) {
	var dialed atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dialed.Add(1)
	}))
	defer srv.Close()

	c := NewDxLinkConn(func() string { return "ws" + strings.TrimPrefix(srv.URL, "http") }, func() string { return "tok" })
	waiting := make(chan struct{})
	c.OnWaitForSessionCooldown(func(ctx context.Context) (func(), error) {
		close(waiting)
		<-ctx.Done()
		return nil, ctx.Err()
	})

	errCh := make(chan error, 1)
	go func() { errCh <- c.Connect(context.Background()) }()

	<-waiting
	c.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from Connect on Close during cooldown")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Connect to abort during cooldown")
	}
	if dialed.Load() != 0 {
		t.Fatalf("expected 0 dials, got %d", dialed.Load())
	}
}
