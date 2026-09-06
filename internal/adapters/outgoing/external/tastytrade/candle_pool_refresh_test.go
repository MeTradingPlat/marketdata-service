package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Con un solo canal, refreshCycle alterna cual mitad le toca -- dos
	// llamadas consecutivas garantizan que ese canal caiga en una de ellas
	// (ver el comentario de refreshCycle en candle_pool.go).
	pool.RefreshLiveSubscriptions(context.Background())
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

// TestRefreshLiveSubscriptions_AlternatesHalvesAndStaggersWithinEach prueba
// las dos salvaguardas juntas: (1) cada llamada solo toca la mitad de los
// canales, alternando cual mitad entre llamadas -- confirmado en vivo el
// 2026-09-05 que mandar TODOS los canales juntos cada minuto disparaba un
// reenganche masivo de sesiones que terminaba saturando el limite de
// TastyTrade -- y (2) el envio dentro de esa mitad sigue espaciado
// (refreshChannelStagger), salvaguarda previa contra el tope de 10,000
// cambios de suscripcion/minuto de dxLink.
func TestRefreshLiveSubscriptions_AlternatesHalvesAndStaggersWithinEach(t *testing.T) {
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

	pc := newPooledConnection(conn)
	for id := 1; id <= 4; id++ {
		ch := newDxLinkChannel(id, conn)
		conn.channels[id] = ch
		pooled := newPooledChannel(ch)
		pooled.occupy(candleKey(fmt.Sprintf("SYM%d", id), domain.M1))
		pc.addChannel(pooled)
	}

	allocator := &channelAllocator{maxConnections: defaultMaxConnections, connections: []*pooledConnection{pc}}
	pool := &CandlePool{allocator: allocator}

	waitFor := func(n int) {
		deadline := time.Now().Add(2 * time.Second)
		for {
			mu.Lock()
			got := len(received)
			mu.Unlock()
			if got >= n || time.Now().After(deadline) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	start := time.Now()
	pool.RefreshLiveSubscriptions(context.Background())
	elapsed := time.Since(start)
	waitFor(2)

	mu.Lock()
	afterFirst := len(received)
	mu.Unlock()
	if afterFirst != 2 {
		t.Fatalf("expected the first call to refresh exactly half (2) of 4 channels, got %d messages", afterFirst)
	}
	if wantMin := 1 * refreshChannelStagger; elapsed < wantMin {
		t.Fatalf("expected refreshing 2 channels to take at least %v (staggered), took %v", wantMin, elapsed)
	}

	pool.RefreshLiveSubscriptions(context.Background())
	waitFor(4)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 4 {
		t.Fatalf("expected the second call to refresh the OTHER half, covering all 4 channels total, got %d messages", len(received))
	}
	seen := map[int]bool{}
	for _, msg := range received {
		seen[msg.Channel] = true
	}
	for id := 1; id <= 4; id++ {
		if !seen[id] {
			t.Fatalf("expected channel %d refreshed across the two alternating calls, got %+v", id, received)
		}
	}
}
