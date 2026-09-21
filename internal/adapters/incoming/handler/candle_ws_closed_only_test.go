package handler

import (
	"encoding/json"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
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

func newTestBaseSession(outCap int, overflowMax int, sender func(any)) *baseWSSession {
	return &baseWSSession{
		out:            make(chan any, outCap),
		done:           make(chan struct{}),
		overflow:       newOverflowQueue(overflowMax),
		overflowSignal: make(chan struct{}, 1),
		sender:         sender,
	}
}

func TestBaseWSSession_UnDesbordeDeVelasCerradasSeEntregaCompletoSinCrearGoroutinesPorMensaje(t *testing.T) {
	const total = 5000
	var mu sync.Mutex
	delivered := make(map[int]int, total)
	s := newTestBaseSession(8, 100_000, func(v any) {
		time.Sleep(20 * time.Microsecond)
		mu.Lock()
		delivered[v.(int)]++
		mu.Unlock()
	})
	go s.dispatchLoop()
	defer close(s.done)
	before := runtime.NumGoroutine()

	for i := 0; i < total; i++ {
		s.publishReliable(i)
	}
	during := runtime.NumGoroutine()

	deadline := time.After(10 * time.Second)
	for {
		mu.Lock()
		n := len(delivered)
		mu.Unlock()
		if n == total {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("se entregaron %d de %d velas cerradas", n, total)
		case <-time.After(10 * time.Millisecond):
		}
	}
	for i, count := range delivered {
		if count != 1 {
			t.Fatalf("la vela %d se entrego %d veces", i, count)
		}
	}
	if during-before > 20 {
		t.Fatalf("el desborde no debe crear una goroutine por mensaje: %d -> %d", before, during)
	}
}

func TestBaseWSSession_ElDesbordeTieneTopeYDescartaElExceso(t *testing.T) {
	s := newTestBaseSession(1, 3, nil)

	for i := 0; i < 10; i++ {
		s.publishReliable(i)
	}

	if got := len(s.overflow.take()); got != 3 {
		t.Fatalf("la cola de desborde debe topear en 3, got %d", got)
	}
	if len(s.out) != 1 {
		t.Fatalf("out debia quedar con 1 mensaje, got %d", len(s.out))
	}
}

func TestBaseWSSession_LasParcialesSeSiguenDescartandoConLaColaLlena(t *testing.T) {
	s := newTestBaseSession(1, 10, nil)

	s.publish("parcial-1")
	s.publish("parcial-2")

	if len(s.out) != 1 || len(s.overflow.take()) != 0 {
		t.Fatalf("una parcial con la cola llena se descarta, no va al desborde")
	}
}

func TestCandleWS_LasVelasCerradasNuevasLlevanNumeroDeSecuenciaConsecutivo(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = time.Hour
	defer func() { aggregateCloseDelay = previous }()
	wsURL, feed := startCandleWSServerWithFeed(t)
	conn := subscribeAndAwaitHistory(t, wsURL,
		candleSubscribeRequest{Action: "subscribe", Symbols: []string{"AAPL"}, Timeframe: "M1", ClosedOnly: true})
	time.Sleep(100 * time.Millisecond)
	base := time.Now().UTC().Truncate(time.Minute)
	candle := func(ts time.Time, volume int64) domain.Candle {
		return domain.Candle{Symbol: "AAPL", Timestamp: ts, Open: 10, High: 11, Low: 9, Close: 10, Volume: volume}
	}

	feed.Publish("AAPL", candle(base, 10))
	feed.Publish("AAPL", candle(base.Add(time.Minute), 20))
	feed.Publish("AAPL", candle(base.Add(2*time.Minute), 30))
	feed.Publish("AAPL", candle(base.Add(time.Minute), 25))

	var seqs []int64
	var correctedSeq int64 = -1
	for {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var env struct {
			Type string `json:"type"`
			Bar  struct {
				Seq       int64 `json:"seq"`
				Corrected bool  `json:"corrected"`
			} `json:"bar"`
		}
		if json.Unmarshal(msg, &env) == nil && env.Type == "bar" {
			if env.Bar.Corrected {
				correctedSeq = env.Bar.Seq
			} else {
				seqs = append(seqs, env.Bar.Seq)
			}
		}
	}

	if len(seqs) != 2 || seqs[0] != 1 || seqs[1] != 2 {
		t.Fatalf("secuencias de las cerradas nuevas = %v, want [1 2]", seqs)
	}
	if correctedSeq != 0 {
		t.Fatalf("una vela corregida no consume secuencia, got seq=%d", correctedSeq)
	}
}
