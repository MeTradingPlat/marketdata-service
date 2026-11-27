package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestDxLinkConn_HandleMessage_DiagnosticsCounters(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	ch, err := c.OpenChannel(context.Background())
	if err != nil {
		t.Fatalf("OpenChannel: %v", err)
	}

	c.stats.recordAbandonedChannel(99, time.Now().Add(-time.Second))
	c.handleMessage(context.Background(), []byte(`{"type":"CHANNEL_OPENED","channel":99,"service":"FEED"}`))
	c.handleMessage(context.Background(), []byte(`{"type":"FEED_CONFIG","channel":99}`))
	c.handleMessage(context.Background(), []byte(`{"type":"CHANNEL_CLOSED","channel":`+string(rune('0'+ch.id))+`}`))
	c.handleMessage(context.Background(), []byte(`{"type":"CHANNEL_CLOSED","channel":99}`))
	c.handleMessage(context.Background(), []byte(`{"type":"FEED_DATA","channel":99,"data":[]}`))
	c.handleMessage(context.Background(), []byte(`{"type":"WEIRD_MSG","channel":0}`))
	c.handleMessage(context.Background(), []byte(`{"type":"WEIRD_MSG","channel":0}`))
	c.handleMessage(context.Background(), []byte(`{"type":"ERROR","channel":3,"error":"BAD_ACTION","message":"bad"}`))
	c.handleMessage(context.Background(), []byte(`{"type":"ERROR","channel":3,"error":"OTHER_ERR","message":"other"}`))

	snap := c.stats.snapshot()
	if snap["late_responses"].(int64) != 2 {
		t.Fatalf("expected late_responses=2, got %v", snap["late_responses"])
	}
	if snap["channels_closed"].(int64) != 2 {
		t.Fatalf("expected channels_closed=2, got %v", snap["channels_closed"])
	}
	errs := snap["server_errors"].(map[string]int64)
	if errs["BAD_ACTION"] != 1 || errs["OTHER_ERR"] != 1 {
		t.Fatalf("unexpected server_errors: %v", errs)
	}
	unk := snap["unknown_types"].(map[string]int64)
	if unk["WEIRD_MSG"] != 2 {
		t.Fatalf("expected WEIRD_MSG=2, got %v", unk)
	}
	if c.channel(ch.id) == nil {
		t.Fatal("expected CHANNEL_CLOSED not to remove channel")
	}
}

func TestDxLinkSocketStats_AbandonedChannelsBounded(t *testing.T) {
	var s dxLinkSocketStats
	base := time.Unix(1000, 0)
	for i := 1; i <= maxAbandonedChannels+5; i++ {
		s.recordAbandonedChannel(i, base.Add(time.Duration(i)*time.Second))
	}
	if _, ok := s.abandonedAge(1, base); ok {
		t.Fatal("expected oldest abandoned channel to be evicted")
	}
	if _, ok := s.abandonedAge(maxAbandonedChannels+5, base); !ok {
		t.Fatal("expected newest abandoned channel to remain")
	}
}
