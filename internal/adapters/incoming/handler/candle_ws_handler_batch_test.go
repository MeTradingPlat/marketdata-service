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
	singleCalls  int
}

func (f *fakeGetCandlesService) GetCandles(_ context.Context, symbol string, _ domain.Timeframe, _ int, _ *time.Time) ([]domain.Candle, error) {
	f.mu.Lock()
	f.singleCalls++
	f.mu.Unlock()
	return []domain.Candle{{Symbol: symbol, Timestamp: time.Unix(1_700_000_000, 0), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}}, nil
}

func (f *fakeGetCandlesService) GetCandlesBatch(_ context.Context, symbols []string, _ domain.Timeframe, _ int) map[string][]domain.Candle {
	f.mu.Lock()
	f.batchCalls++
	f.batchSymbols = append(f.batchSymbols, symbols...)
	f.mu.Unlock()
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
