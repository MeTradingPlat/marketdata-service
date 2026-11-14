package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// FetchHistoryDeep es el probe de profundidad maxima -- misma logica que
// FetchHistory pero con historyDeepWait: sin un watermark que acote, la
// rafaga historica completa de un simbolo profundo puede superar los 15s
// del fetch incremental (ver historyDeepWait).
func (p *CandlePool) FetchHistoryDeep(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return p.fetchHistory(ctx, symbol, tf, from, historyDeepWait)
}

func (p *CandlePool) FetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return p.fetchHistory(ctx, symbol, tf, from, historyDefaultWait)
}

// FetchHistoryWithWait expone fetchHistory con un wait a eleccion -- solo
// para cmd/verify-depth (confirmar en vivo cuanto tarda de verdad la
// rafaga historica de un simbolo puntual, sin el limite de historyDeepWait
// de por medio).
func (p *CandlePool) FetchHistoryWithWait(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time, wait time.Duration) ([]domain.Candle, error) {
	return p.fetchHistory(ctx, symbol, tf, from, wait)
}

func (p *CandlePool) fetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time, wait time.Duration) ([]domain.Candle, error) {
	// Un fetch M1 puntual para un simbolo que YA esta en vivo competiria por
	// la MISMA suscripcion server-side (dxFeed fusiona el "add" de esta
	// peticion con el "add" en vivo en un solo tema), y el "remove" del
	// cleanup de este fetch se lleva TAMBIEN la suscripcion en vivo por
	// delante -- confirmado en vivo: el streaming de simbolos que ademas
	// caian dentro de un lote de backfill se quedaba mudo para siempre, sin
	// ningun error, porque el TCP seguia perfectamente sano sirviendo el
	// resto del trafico del lote. El stream en vivo (mas el relleno de
	// huecos tras reconexion) ya es la fuente autoritativa hacia adelante
	// para M1, asi que aqui no hay nada que este fetch pueda aportar.
	if tf == domain.M1 && p.LiveSubscribed(symbol) {
		return nil, nil
	}

	key := candleKey(symbol, tf)

	lockVal, _ := p.historyLocks.LoadOrStore(key, &sync.Mutex{})
	lock := lockVal.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocating channel for %s %s history: %w", symbol, tf, err)
	}

	ch.occupy(key)

	collector := newHistoryCollector(symbol, tf)
	dispatchID := p.registerDispatch(symbol, tf, "history", collector.onCandle)

	// El remove va primero (le da al servidor la maxima ventaja de tiempo
	// para procesarlo), el dispatch se borra despues de un rato -- ver
	// unsubscribeDrainPeriod. unregisterDispatchIfCurrent solo borra si
	// nadie volvio a registrar esa misma clave mientras tanto.
	cleanup := func() {
		ch.release(key)
		_ = ch.channel.unsubscribe(symbol, tf)
		time.AfterFunc(unsubscribeDrainPeriod, func() {
			p.unregisterDispatchIfCurrent(symbol, tf, dispatchID, "history")
		})
	}

	if err := ch.channel.subscribeHistory(symbol, tf, from); err != nil {
		cleanup()
		return nil, fmt.Errorf("subscribing history: %w", err)
	}
	if err := waitForData(ctx, collector.settled, wait); err != nil {
		cleanup()
		if errors.Is(err, domain.ErrPartialHistory) {
			return collector.complete(), err
		}
		return nil, err
	}
	result := collector.complete()
	cleanup()
	return result, nil
}
