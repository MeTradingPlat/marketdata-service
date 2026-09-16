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
// cuantos suscriptores tenga: el primero crea el worker (con su propia
// goroutine leyendo M1 crudo del Broadcaster existente), los siguientes lo
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

type aggregateWorker struct {
	out      *livecandles.Broadcaster[dto.CandleBar]
	stopRaw  func()
	refCount int
}

// Subscribe engancha una sesion a la agregacion compartida de
// (symbol, timeframe). refCount decide cuando el worker de verdad arranca
// (primer suscriptor) y cuando se apaga (ultimo que se va) -- protegido por
// el mismo mutex que el mapa de workers para que un alta y una baja
// simultaneas del mismo par nunca se pisen.
func (h *candleAggregateHub) Subscribe(ctx context.Context, symbol, timeframe string, tf domain.Timeframe) (ch <-chan dto.CandleBar, cancel func()) {
	key := symbol + ":" + timeframe

	h.mu.Lock()
	w, exists := h.workers[key]
	if !exists {
		rawCh, stopRaw := h.raw.Subscribe(symbol)
		w = &aggregateWorker{out: livecandles.NewBroadcaster[dto.CandleBar](), stopRaw: stopRaw}
		h.workers[key] = w
		var seed *dto.CandleBar
		if h.current != nil {
			seed, _ = h.current.GetCurrentCandle(ctx, symbol, tf)
		}
		go w.run(rawCh, tf, seedAggregate(seed, tf, time.Now()))
	}
	w.refCount++
	h.mu.Unlock()

	outCh, cancelOut := w.out.Subscribe(aggregateBroadcastKey)
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
	return outCh, cancel
}

// run agrega el M1 crudo al periodo de tf -- misma logica que el viejo
// wsSession.forwardLive tenia por sesion, ahora corrida una sola vez por
// (symbol, timeframe) y publicada a quien este suscripto via `out`.
func (w *aggregateWorker) run(ch <-chan domain.Candle, tf domain.Timeframe, agg *dto.CandleBar) {
	for c := range ch {
		// Un tick puede traer OHLC parcial (minuto sin trades, primer evento
		// de un periodo) -- se descarta, el siguiente tick lo completa.
		if c.Close == 0 {
			continue
		}
		period := livecandles.FormingPeriodStart(c.Timestamp, tf).Unix()
		if agg == nil || agg.Time != period {
			if agg != nil {
				closedBar := *agg
				closedBar.Closed = true
				w.out.Publish(aggregateBroadcastKey, closedBar)
			}
			agg = &dto.CandleBar{Time: period, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume, Closed: false}
		} else {
			if c.High > agg.High {
				agg.High = c.High
			}
			if c.Low < agg.Low {
				agg.Low = c.Low
			}
			agg.Close = c.Close
			agg.Volume += c.Volume
		}
		w.out.Publish(aggregateBroadcastKey, *agg)
	}
}
