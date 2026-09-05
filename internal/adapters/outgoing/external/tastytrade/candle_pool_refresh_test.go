package tastytrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/gorilla/websocket"
)

// TestRefreshLiveSubscriptions_ResendsAddWithoutOpeningNewChannelOrConnection
// prueba el diseño pedido: cada canal reenvia el Add de sus propios simbolos
// M1 por su misma conexion, sin abrir ningun canal o conexion nueva (ver el
// comentario de subscribeLive sobre por que un remove+add mataria el stream).
func TestRefreshLiveSubscriptions_ResendsAddWithoutOpeningNewChannelOrConnection(t *testing.T) {
	var mu sync.Mutex
	var received []feedSubscriptionMessage

	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg feedSubscriptionMessage
			if err := json.Unmarshal(raw, &msg); err == nil && msg.Type == "FEED_SUBSCRIPTION" {
				mu.Lock()
				received = append(received, msg)
				mu.Unlock()
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dialing test server: %v", err)
	}

	conn := NewDxLinkConn(func() string { return wsURL }, func() string { return "token" })
	conn.conn = dialConn
	conn.authenticated = true
	conn.channels = make(map[int]*dxLinkChannel)

	ch := newDxLinkChannel(1, conn)
	conn.channels[1] = ch
	pooled := newPooledChannel(ch)
	pooled.occupy(candleKey("AAPL", domain.M1))
	pooled.occupy(candleKey("MSFT", domain.M1))
	pooled.occupy(candleKey("SPY", domain.D1)) // historial, no en vivo -- no debe refrescarse

	pc := newPooledConnection(conn)
	pc.addChannel(pooled)

	allocator := &channelAllocator{maxConnections: defaultMaxConnections, connections: []*pooledConnection{pc}}
	pool := &CandlePool{allocator: allocator}

	pool.RefreshLiveSubscriptions(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected exactly 1 FEED_SUBSCRIPTION message, got %d", len(received))
	}
	msg := received[0]
	if msg.Channel != 1 {
		t.Fatalf("expected the message on the existing channel 1, got channel %d", msg.Channel)
	}
	if len(msg.Add) != 2 {
		t.Fatalf("expected 2 refreshed symbols (only the M1-live ones), got %d: %+v", len(msg.Add), msg.Add)
	}
	got := map[string]bool{}
	for _, item := range msg.Add {
		got[item.Symbol] = true
		if item.FromTime == nil {
			t.Fatalf("expected FromTime set on refresh item %+v", item)
		}
	}
	if !got["AAPL{=1m}"] || !got["MSFT{=1m}"] {
		t.Fatalf("expected AAPL and MSFT refreshed, got %+v", got)
	}

	if got := allocator.connectionCount(); got != 1 {
		t.Fatalf("expected no new connection to be opened, connection count = %d", got)
	}
	pc.mu.Lock()
	channelCount := len(pc.channels)
	pc.mu.Unlock()
	if channelCount != 1 {
		t.Fatalf("expected no new channel to be opened, channel count = %d", channelCount)
	}
}

// TestRefreshLiveSubscriptions_StaggersBetweenChannels prueba que el envio
// no sale todo en la misma fraccion de segundo -- TastyTrade publica un tope
// de 10,000 cambios de suscripcion por minuto para dxLink sin aclarar si
// cuenta por mensaje o por simbolo, asi que espaciar el envio entre canales
// es la unica salvaguarda posible sin confirmarlo contra la cuenta real.
func TestRefreshLiveSubscriptions_StaggersBetweenChannels(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dialing test server: %v", err)
	}

	conn := NewDxLinkConn(func() string { return wsURL }, func() string { return "token" })
	conn.conn = dialConn
	conn.authenticated = true
	conn.channels = make(map[int]*dxLinkChannel)

	pc := newPooledConnection(conn)
	for id, symbol := range map[int]string{1: "AAPL", 2: "MSFT", 3: "NVDA"} {
		ch := newDxLinkChannel(id, conn)
		conn.channels[id] = ch
		pooled := newPooledChannel(ch)
		pooled.occupy(candleKey(symbol, domain.M1))
		pc.addChannel(pooled)
	}

	allocator := &channelAllocator{maxConnections: defaultMaxConnections, connections: []*pooledConnection{pc}}
	pool := &CandlePool{allocator: allocator}

	start := time.Now()
	pool.RefreshLiveSubscriptions(context.Background())
	elapsed := time.Since(start)

	wantMin := 2 * refreshChannelStagger // 3 canales = 2 pausas entre ellos
	if elapsed < wantMin {
		t.Fatalf("expected refreshing 3 channels to take at least %v (staggered), took %v", wantMin, elapsed)
	}
}
