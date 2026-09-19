package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// fakeGetCandlesService cuenta cuantas veces se llama GetCandlesBatch (para
// confirmar que una suscripcion en lote hace UNA sola consulta, no una por
// simbolo) y devuelve una vela minima por simbolo pedido.
type fakeGetCandlesService struct {
	mu           sync.Mutex
	batchCalls   int
	batchSymbols []string
	batchBars    []int
	singleCalls  int
	// batchDelay simula un GetCandlesBatch lento (universo grande, DB bajo
	// carga) -- usado para probar que la sesion sigue respondiendo a otra
	// cosa (ej. un PING) mientras este esta en vuelo.
	batchDelay time.Duration
}

func (f *fakeGetCandlesService) GetCandles(_ context.Context, symbol string, _ domain.Timeframe, _ int, _ *time.Time) ([]domain.Candle, error) {
	f.mu.Lock()
	f.singleCalls++
	f.mu.Unlock()
	return []domain.Candle{{Symbol: symbol, Timestamp: time.Unix(1_700_000_000, 0), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}}, nil
}

func (f *fakeGetCandlesService) GetCandlesBatch(_ context.Context, symbols []string, _ domain.Timeframe, bars int) map[string][]domain.Candle {
	f.mu.Lock()
	f.batchCalls++
	f.batchBars = append(f.batchBars, bars)
	f.batchSymbols = append(f.batchSymbols, symbols...)
	delay := f.batchDelay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	result := make(map[string][]domain.Candle, len(symbols))
	for _, symbol := range symbols {
		result[symbol] = []domain.Candle{{Symbol: symbol, Timestamp: time.Unix(1_700_000_000, 0), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}}
	}
	return result
}

func startCandleWSServer(t *testing.T) (wsURL string, fake *fakeGetCandlesService) {
	t.Helper()
	fake = &fakeGetCandlesService{}
	broadcaster := livecandles.NewBroadcaster[domain.Candle]()
	h := NewCandleWSHandler(fake, nilCurrentCandleService{}, broadcaster)

	e := echo.New()
	e.GET("/ws/candles", h.Handle)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/candles", fake
}

func TestHandleSubscribeBatch_UnaSolaConsultaParaVariosSimbolos(t *testing.T) {
	wsURL, fake := startCandleWSServer(t)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	req := candleSubscribeRequest{Action: "subscribe", Symbols: []string{"AAPL", "MSFT", "TSLA"}, Timeframe: "M5"}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got := map[string]bool{}
	for len(got) < 3 {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read failed waiting for %d/3 history messages: %v", len(got), err)
		}
		var env struct {
			Type   string `json:"type"`
			Symbol string `json:"symbol"`
		}
		if err := json.Unmarshal(msg, &env); err != nil {
			continue
		}
		if env.Type == "history" {
			got[env.Symbol] = true
		}
	}

	fake.mu.Lock()
	batchCalls, singleCalls := fake.batchCalls, fake.singleCalls
	fake.mu.Unlock()
	if batchCalls != 1 {
		t.Fatalf("batchCalls = %d, want 1 (una sola consulta para los 3 simbolos)", batchCalls)
	}
	if singleCalls != 0 {
		t.Fatalf("singleCalls = %d, want 0 (no debe caer al camino individual)", singleCalls)
	}
	for _, symbol := range []string{"AAPL", "MSFT", "TSLA"} {
		if !got[symbol] {
			t.Fatalf("no llego historial para %s", symbol)
		}
	}
}

func TestHandleSubscribeBatch_NoBloqueaElLoopDeLecturaMientrasCargaElHistorial(t *testing.T) {
	fake := &fakeGetCandlesService{batchDelay: 500 * time.Millisecond}
	broadcaster := livecandles.NewBroadcaster[domain.Candle]()
	h := NewCandleWSHandler(fake, nilCurrentCandleService{}, broadcaster)
	e := echo.New()
	e.GET("/ws/candles", h.Handle)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/candles"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	req := candleSubscribeRequest{Action: "subscribe", Symbols: []string{"AAPL", "MSFT", "TSLA"}, Timeframe: "M5"}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// El batch subscribe recien pedido tarda 500ms del lado del servidor
	// (fake.batchDelay) -- si el handler corriera en la MISMA goroutine que
	// lee del socket (el bug real, confirmado en vivo 2026-09-16), un PING
	// mandado ahora quedaria sin PONG hasta que ese fetch termine. Con el
	// fix (handler en su propia goroutine), el servidor sigue respondiendo
	// pings de inmediato aunque el fetch siga en vuelo.
	pongReceived := make(chan struct{}, 1)
	conn.SetPongHandler(func(string) error {
		select {
		case pongReceived <- struct{}{}:
		default:
		}
		return nil
	})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	time.Sleep(50 * time.Millisecond) // deja que el subscribe arranque su fetch lento
	if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
		t.Fatalf("failed to send ping: %v", err)
	}

	select {
	case <-pongReceived:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no llego el pong dentro de 200ms -- el fetch lento del batch subscribe esta bloqueando el loop de lectura")
	}
}

func TestHandleSubscribeBatch_SesionCerradaMientrasElFetchSigueEnVuelo(t *testing.T) {
	// Regresion: si el cliente se desconecta (closeAll corre, nilea s.subs)
	// ANTES de que un handleSubscribeBatch lanzado en su propia goroutine
	// termine su fetch lento, ese handler no debe panicar al intentar
	// guardar la suscripcion -- confirmado en CI 2026-09-16 (panic:
	// assignment to entry in nil map, tumbaba el proceso entero).
	fake := &fakeGetCandlesService{batchDelay: 150 * time.Millisecond}
	broadcaster := livecandles.NewBroadcaster[domain.Candle]()
	h := NewCandleWSHandler(fake, nilCurrentCandleService{}, broadcaster)
	e := echo.New()
	e.GET("/ws/candles", h.Handle)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/candles"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	req := candleSubscribeRequest{Action: "subscribe", Symbols: []string{"AAPL"}, Timeframe: "M5"}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	// Cierra del lado del cliente casi de inmediato -- el fetch de 150ms
	// del fake sigue corriendo en su propia goroutine del lado del
	// servidor cuando esto pasa.
	conn.Close()

	// Si el servidor panicara en esa goroutine, se lleva el proceso de
	// test entero (no aparece como un t.Fatal normal) -- llegar hasta aca
	// sin abortar ya es la asercion real.
	time.Sleep(300 * time.Millisecond)
}

func subscribeBatchAndAwaitHistory(t *testing.T, req candleSubscribeRequest) *fakeGetCandlesService {
	t.Helper()
	wsURL, fake := startCandleWSServer(t)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read failed waiting for history: %v", err)
	}
	return fake
}

func TestHandleSubscribeBatch_UsaLasBarrasQuePideElCliente(t *testing.T) {
	cases := map[string]struct{ requested, want int }{
		"pedido explicito":         {requested: 120, want: 120},
		"sin pedido usa default":   {requested: 0, want: defaultHistoryBars},
		"pedido excesivo se acota": {requested: 1_000_000, want: maxHistoryBars},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := subscribeBatchAndAwaitHistory(t, candleSubscribeRequest{
				Action: "subscribe", Symbols: []string{"AAPL"}, Timeframe: "M5", Bars: tc.requested,
			})
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.batchBars) != 1 || fake.batchBars[0] != tc.want {
				t.Fatalf("batchBars = %v, want [%d]", fake.batchBars, tc.want)
			}
		})
	}
}
