package tastytrade

import (
	"context"
	"testing"
)

func TestDxLinkConn_CloseReason_IntentionalClose(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	if _, err := c.OpenChannel(context.Background()); err != nil {
		t.Fatalf("OpenChannel: %v", err)
	}
	if c.stats.opensSucceeded.Load() != 1 {
		t.Fatalf("expected opensSucceeded=1, got %d", c.stats.opensSucceeded.Load())
	}
	c.Close()
	if c.closeReasonText() != "intentional_close" {
		t.Fatalf("expected intentional_close, got %q", c.closeReasonText())
	}
}

func TestDxLinkConn_PublishConn_ResetsCloseReasonAndStats(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
	t.Cleanup(c.Close)
	c.noteCloseReason("stale_reason")
	c.stats.opensSucceeded.Add(1)
	if c.closeReasonText() != "stale_reason" {
		t.Fatalf("expected stale_reason before Connect, got %q", c.closeReasonText())
	}
	if c.stats.opensSucceeded.Load() != 1 {
		t.Fatalf("expected opensSucceeded=1 before Connect, got %d", c.stats.opensSucceeded.Load())
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.closeReasonText() != "" {
		t.Fatalf("expected empty closeReason after publish, got %q", c.closeReasonText())
	}
	if c.stats.opensSucceeded.Load() != 0 {
		t.Fatalf("expected stats reset to 0, got %d", c.stats.opensSucceeded.Load())
	}
}
