package handler

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

const aggregateBroadcastKey = "agg"

// candleAggregateHub comparte, entre TODAS las sesiones WS de /ws/candles
// que pidan el mismo (symbol, timeframe), el trabajo de agregar M1 -> ese
// timeframe -- antes CADA sesion (wsSession.forwardLive) corria su PROPIA
// copia de esta agregacion sobre el mismo M1 crudo, duplicando el trabajo
// tantas veces como escaneres/clientes pidieran el mismo par. Con esto la
// agregacion corre una sola vez por (symbol, timeframe), sin importar
// cuantos suscriptores tenga: el primero crea el worker, los siguientes lo
// reusan, y se apaga solo cuando el ultimo se desuscribe.
type candleAggregateHub struct {
	raw     *livecandles.Broadcaster[domain.Candle]
	current in.GetCurrentCandleService

	mu      sync.Mutex
	workers map[string]*aggregateWorker
}

func newCandleAggregateHub(raw *livecandles.Broadcaster[domain.Candle], current in.GetCurrentCandleService) *candleAggregateHub {
	return &candleAggregateHub{raw: raw, current: current, workers: make(map[string]*aggregateWorker)}
}

// Subscribe engancha push a la agregacion compartida de (symbol, timeframe).
// refCount decide cuando el worker de verdad arranca (primer suscriptor) y
// cuando se apaga (ultimo que se va) -- protegido por el mismo mutex que el
// mapa de workers para que un alta y una baja simultaneas del mismo par
// nunca se pisen. push corre en la goroutine del publicador del tick M1
// (ver comentario de Broadcaster): debe ser rapido y no bloqueante.
func (h *candleAggregateHub) Subscribe(ctx context.Context, symbol, timeframe string, tf domain.Timeframe, push func(dto.CandleBar)) (cancel func()) {
	key := symbol + ":" + timeframe

	h.mu.Lock()
	w, exists := h.workers[key]
	if !exists {
		var seed *dto.CandleBar
		var seedMinuteVolume int64
		if h.current != nil {
			seed, _ = h.current.GetCurrentCandle(ctx, symbol, tf)
			seedMinuteVolume = h.formingMinuteVolume(ctx, symbol)
		}
		w = newAggregateWorker(tf, seedAggregate(seed, tf, time.Now()), seedMinuteVolume)
		w.stopRaw = h.raw.Subscribe(symbol, w.onTick)
		h.workers[key] = w
	}
	w.refCount++
	h.mu.Unlock()

	cancelOut := w.out.Subscribe(aggregateBroadcastKey, push)
	cancel = func() {
		cancelOut()
		h.mu.Lock()
		w.refCount--
		if w.refCount == 0 {
			delete(h.workers, key)
			w.stopRaw()
			w.stop()
		}
		h.mu.Unlock()
	}
	return cancel
}

func (h *candleAggregateHub) formingMinuteVolume(ctx context.Context, symbol string) int64 {
	minute, err := h.current.GetCurrentCandle(ctx, symbol, domain.M1)
	if err != nil || minute == nil || minute.Time != time.Now().UTC().Truncate(time.Minute).Unix() {
		return 0
	}
	return minute.Volume
}
