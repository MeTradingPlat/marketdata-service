package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

func startCandleWSServerWithFeed(t *testing.T) (string, *livecandles.Broadcaster[domain.Candle]) {
	t.Helper()
	broadcaster := livecandles.NewBroadcaster[domain.Candle]()
	h := NewCandleWSHandler(&fakeGetCandlesService{}, nilCurrentCandleService{}, broadcaster)
	e := echo.New()
	e.GET("/ws/candles", h.Handle)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/candles", broadcaster
}

func subscribeAndAwaitHistory(t *testing.T, wsURL string, req candleSubscribeRequest) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read history failed: %v", err)
	}
	return conn
}

func readBars(conn *websocket.Conn, wait time.Duration) []bool {
	var closedFlags []bool
	for {
		conn.SetReadDeadline(time.Now().Add(wait))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return closedFlags
		}
		var env struct {
			Type string `json:"type"`
			Bar  struct {
				Closed bool `json:"closed"`
			} `json:"bar"`
		}
		if json.Unmarshal(msg, &env) == nil && env.Type == "bar" {
			closedFlags = append(closedFlags, env.Bar.Closed)
		}
	}
}

func publishTwoMinutes(feed *livecandles.Broadcaster[domain.Candle]) {
	base := time.Now().UTC().Truncate(time.Minute)
	tick := func(ts time.Time, volume int64) domain.Candle {
		return domain.Candle{Symbol: "AAPL", Timestamp: ts, Open: 10, High: 11, Low: 9, Close: 10, Volume: volume}
	}
	feed.Publish("AAPL", tick(base, 40))
	feed.Publish("AAPL", tick(base, 100))
	feed.Publish("AAPL", tick(base.Add(time.Minute), 30))
}

func TestCandleWS_ClosedOnlyRecibeSoloLasVelasCerradas(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = time.Hour
	defer func() { aggregateCloseDelay = previous }()
	wsURL, feed := startCandleWSServerWithFeed(t)
	conn := subscribeAndAwaitHistory(t, wsURL,
		candleSubscribeRequest{Action: "subscribe", Symbols: []string{"AAPL"}, Timeframe: "M1", ClosedOnly: true})
	time.Sleep(100 * time.Millisecond)

	publishTwoMinutes(feed)

	got := readBars(conn, 500*time.Millisecond)
	if len(got) != 1 || !got[0] {
		t.Fatalf("closedOnly debe recibir exactamente 1 vela y cerrada, got %v", got)
	}
}

func TestCandleWS_SinClosedOnlyElGraficoSigueRecibiendoLasVelasEnFormacion(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = time.Hour
	defer func() { aggregateCloseDelay = previous }()
	wsURL, feed := startCandleWSServerWithFeed(t)
	conn := subscribeAndAwaitHistory(t, wsURL,
		candleSubscribeRequest{Action: "subscribe", Symbols: []string{"AAPL"}, Timeframe: "M1"})
	time.Sleep(100 * time.Millisecond)

	publishTwoMinutes(feed)

	got := readBars(conn, 500*time.Millisecond)
	if len(got) != 4 || got[0] || got[1] || !got[2] || got[3] {
		t.Fatalf("sin closedOnly deben llegar 2 parciales, la cerrada y 1 parcial nueva; got %v", got)
	}
}

func TestBaseWSSession_UnaVelaCerradaConLaColaLlenaSeRetrasaPeroNoSePierde(t *testing.T) {
	s := &baseWSSession{out: make(chan any, 1), done: make(chan struct{})}
	s.publish("parcial-1")

	s.publish("parcial-2")
	s.publishReliable("cerrada")

	if got := <-s.out; got != "parcial-1" {
		t.Fatalf("primero debia salir lo ya encolado, got %v", got)
	}
	select {
	case got := <-s.out:
		if got != "cerrada" {
			t.Fatalf("debia llegar la cerrada (el parcial-2 se descarta), got %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("la vela cerrada se perdio con la cola llena")
	}
}
