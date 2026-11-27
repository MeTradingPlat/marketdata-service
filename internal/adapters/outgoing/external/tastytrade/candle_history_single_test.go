package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/gorilla/websocket"
)

func startSubCountingDxLinkServer(t *testing.T) (string, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var adds, removes atomic.Int64
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env struct {
				Type    string            `json:"type"`
				Channel int               `json:"channel"`
				Add     []json.RawMessage `json:"add"`
				Remove  []json.RawMessage `json:"remove"`
			}
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			switch env.Type {
			case "SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
			case "AUTH":
				_ = conn.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
			case "CHANNEL_REQUEST":
				_ = conn.WriteJSON(map[string]any{"type": "CHANNEL_OPENED", "channel": env.Channel, "service": "FEED"})
			case "FEED_SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "FEED_CONFIG", "channel": env.Channel})
			case "FEED_SUBSCRIPTION":
				adds.Add(int64(len(env.Add)))
				removes.Add(int64(len(env.Remove)))
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), &adds, &removes
}

func waitForLimiterWaiter(t *testing.T, lim *historySubLimiter) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lim.mu.Lock()
		waiting := lim.notifyCh != nil
		lim.mu.Unlock()
		if waiting {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("timed out waiting for history limiter waiter")
}

func TestFetchHistory_SkipsLiveM1(t *testing.T) {
	p := newTestPool()
	p.liveSubs["AAPL"] = func(domain.Candle) {}

	got, err := p.FetchHistory(context.Background(), "AAPL", domain.M1, time.Now().Add(-time.Hour))
	if err != nil || got != nil {
		t.Fatalf("FetchHistory = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestFetchHistoryWithWait_CancelledContext(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.FetchHistoryWithWait(ctx, "AAPL", domain.D1, time.Now().Add(-time.Hour), 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFetchHistory_SkipsLiveM1DuringLimiterWait(t *testing.T) {
	wsURL, adds, removes := startSubCountingDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := NewCandlePool(factory, 1)
	p.historyLimiter = newHistorySubLimiterWithClock(1, 60*time.Second, time.Now)

	ctx := context.Background()
	blocker, err := p.historyLimiter.acquire(ctx, 1)
	if err != nil {
		t.Fatalf("acquire blocker: %v", err)
	}

	type result struct {
		candles []domain.Candle
		err     error
	}
	done := make(chan result, 1)
	go func() {
		candles, err := p.FetchHistoryWithWait(ctx, "NVDA", domain.M1, time.Now().Add(-time.Hour), 50*time.Millisecond)
		done <- result{candles: candles, err: err}
	}()

	waitForLimiterWaiter(t, p.historyLimiter)
	p.liveMu.Lock()
	p.liveSubs["NVDA"] = func(domain.Candle) {}
	p.liveMu.Unlock()
	blocker.release()

	res := <-done
	if res.err != nil || res.candles != nil {
		t.Fatalf("FetchHistoryWithWait = (%v, %v), want (nil, nil)", res.candles, res.err)
	}
	if adds.Load() != 0 || removes.Load() != 0 {
		t.Fatalf("expected 0 adds and 0 removes, got adds=%d removes=%d", adds.Load(), removes.Load())
	}
	p.historyLimiter.mu.Lock()
	reserved := p.historyLimiter.reserved
	p.historyLimiter.mu.Unlock()
	if reserved != 0 {
		t.Fatalf("expected reserved=0, got %d", reserved)
	}
}

func TestFetchHistory_RegisterDispatchZeroSkipsProviderAndReleasesNonLiveSlot(t *testing.T) {
	tests := []struct {
		name               string
		hostOnOtherChannel bool
		wantKeptOnNewChan  bool
	}{
		{name: "releases slot when another channel hosts live", hostOnOtherChannel: true, wantKeptOnNewChan: false},
		{name: "keeps slot when allocated channel hosts live", hostOnOtherChannel: false, wantKeptOnNewChan: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsURL, adds, removes := startSubCountingDxLinkServer(t)
			baseFactory, _ := newTestConnFactory(t, wsURL)
			var p *CandlePool
			var otherCh *pooledChannel
			var calls atomic.Int32
			factory := func(ctx context.Context) (*DxLinkConn, error) {
				if calls.Add(1) == 2 {
					if tt.hostOnOtherChannel {
						otherCh.occupy(candleKey("NVDA", domain.M1))
					}
					p.liveMu.Lock()
					p.liveSubs["NVDA"] = func(domain.Candle) {}
					p.liveMu.Unlock()
					p.registerDispatch("NVDA", domain.M1, "live", func(rawCandleEvent) {})
				}
				return baseFactory(ctx)
			}
			p = NewCandlePoolWithChannels(factory, 2, 1)

			ctx := context.Background()
			var err error
			otherCh, err = p.allocator.allocate(ctx)
			if err != nil {
				t.Fatalf("warmup allocate: %v", err)
			}
			for i := 0; i < channelCapacity; i++ {
				otherCh.occupy(fmt.Sprintf("FILL%03d", i))
			}

			got, err := p.FetchHistoryWithWait(ctx, "NVDA", domain.M1, time.Now().Add(-time.Hour), 50*time.Millisecond)
			if err != nil || got != nil {
				t.Fatalf("FetchHistoryWithWait = (%v, %v), want (nil, nil)", got, err)
			}
			if adds.Load() != 0 || removes.Load() != 0 {
				t.Fatalf("expected 0 adds and 0 removes, got adds=%d removes=%d", adds.Load(), removes.Load())
			}

			var newCh *pooledChannel
			for _, ch := range p.allocator.allChannels() {
				if ch != otherCh {
					newCh = ch
					break
				}
			}
			if newCh == nil {
				t.Fatal("expected new channel to be allocated")
			}
			if has := channelHasKey(newCh, candleKey("NVDA", domain.M1)); has != tt.wantKeptOnNewChan {
				t.Fatalf("channelHasKey(newCh, NVDA:M1) = %v, want %v", has, tt.wantKeptOnNewChan)
			}
		})
	}
}
