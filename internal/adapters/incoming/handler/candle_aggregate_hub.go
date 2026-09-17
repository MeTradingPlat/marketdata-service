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

// aggregateWorker ya no corre en su propia goroutine -- onTick se llama
// sincronicamente desde la goroutine que publica el tick M1 crudo (ver
// Broadcaster), protegido por su propio mutex porque esa publicacion puede
// venir de mas de un origen concurrente para el mismo simbolo. Antes cada
// worker ocupaba una goroutine bloqueada en un for-range para siempre
// mientras tuviera al menos un suscriptor -- con un escaner sin pre-filtros
// suscribiendo el universo completo en un timeframe, eso eran miles de
// goroutines paradas, la causa real del OOM confirmado en vivo el
// 2026-09-17 (ver comentario de Broadcaster).
type aggregateWorker struct {
	mu       sync.Mutex
	agg      *dto.CandleBar
	tf       domain.Timeframe
	out      *livecandles.Broadcaster[dto.CandleBar]
	stopRaw  func()
	refCount int
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
		if h.current != nil {
			seed, _ = h.current.GetCurrentCandle(ctx, symbol, tf)
		}
		w = &aggregateWorker{out: livecandles.NewBroadcaster[dto.CandleBar](), tf: tf, agg: seedAggregate(seed, tf, time.Now())}
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
		}
		h.mu.Unlock()
	}
	return cancel
}

// onTick agrega el M1 crudo al periodo de tf -- misma logica que el viejo
// aggregateWorker.run tenia en su propio loop, ahora invocada directamente
// por el publicador en vez de leerla de un canal propio.
func (w *aggregateWorker) onTick(c domain.Candle) {
	// Un tick puede traer OHLC parcial (minuto sin trades, primer evento
	// de un periodo) -- se descarta, el siguiente tick lo completa.
	if c.Close == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	period := livecandles.FormingPeriodStart(c.Timestamp, w.tf).Unix()
	if w.agg == nil || w.agg.Time != period {
		if w.agg != nil {
			closedBar := *w.agg
			closedBar.Closed = true
			w.out.Publish(aggregateBroadcastKey, closedBar)
		}
		w.agg = &dto.CandleBar{Time: period, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Closed: false}
	} else {
		if c.High > w.agg.High {
			w.agg.High = c.High
		}
		if c.Low < w.agg.Low {
			w.agg.Low = c.Low
		}
		w.agg.Close = c.Close
		w.agg.Volume += c.Volume
	}
	w.out.Publish(aggregateBroadcastKey, *w.agg)
}
