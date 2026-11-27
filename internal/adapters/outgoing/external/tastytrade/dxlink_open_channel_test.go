package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestDxLinkConn_OpenChannel_CanceledContextSendsNothing(t *testing.T) {
	wsURL, msgs, mu, _ := startRecordingDxLinkServer(t, true)
	c := newConnectedDxLinkConn(t, wsURL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.OpenChannel(ctx); err == nil {
		t.Fatal("expected error for canceled context")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, m := range *msgs {
		if m.Type == "CHANNEL_REQUEST" {
			t.Fatalf("unexpected CHANNEL_REQUEST on canceled context: %+v", m)
		}
	}
}

func TestDxLinkConn_OpenChannel_TimeoutSendsChannelCancel(t *testing.T) {
	wsURL, msgs, mu, cancelCh := startRecordingDxLinkServer(t, false)
	c := newConnectedDxLinkConn(t, wsURL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.OpenChannel(ctx); err == nil {
		t.Fatal("expected error on OpenChannel timeout")
	}
	var cancelMsg channelCancelMessage
	select {
	case cancelMsg = <-cancelCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CHANNEL_CANCEL")
	}
	mu.Lock()
	defer mu.Unlock()
	var reqID int
	for _, m := range *msgs {
		if m.Type == "CHANNEL_REQUEST" {
			reqID = m.Channel
		}
	}
	if reqID == 0 || cancelMsg.Channel != reqID {
		t.Fatalf("expected CHANNEL_CANCEL for channel %d, got %d", reqID, cancelMsg.Channel)
	}
}
