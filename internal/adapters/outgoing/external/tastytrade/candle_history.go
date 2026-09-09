package tastytrade

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	historyDefaultWait = 15 * time.Second
	// historyDeepWait es para el probeo de profundidad MAXIMA (sin watermark
	// todavia, p. ej. FetchHistoryDeep): una rafaga de miles de velas D1/H1
	// puede tardar mas que los 15s del fetch incremental de 10 dias --
	// confirmado en vivo: la espera corta cortaba la historia de simbolos
	// con mucha profundidad y dejaba el backfill truncado para siempre (el
	// incremental solo trae barras NUEVAS, nunca re-probea hacia atras).
	historyDeepWait = 90 * time.Second

	// historyBatchDeepGapThreshold: si algun simbolo del lote necesita
	// ponerse al dia con mas de esto, el lote entero espera historyDeepWait
	// en vez de historyDefaultWait -- confirmado en vivo el 2026-08-19:
	// FetchHistoryBatch (el barrido de RunSweepPhase) nunca tuvo el
	// equivalente de este ajuste que ya existe para el fetch individual (ver
	// historyDeepWait arriba). Con watermarks atrasados varias horas (ej.
	// tras un simbolo silenciosamente estancado unos dias), el lote se
	// asentaba a los 15s con lo que hubiera llegado hasta ahi, guardaba esa
	// porcion parcial como verified=true y avanzaba el watermark hasta ese
	// punto -- el resto del hueco (desde la apertura hasta donde se corto)
	// quedaba para siempre con los datos provisionales del stream en vivo,
	// porque el proximo fetch incremental nunca vuelve a mirar atras del
	// watermark ya avanzado (confirmado en real: SW/SFM/DNTH/GKOS con
	// decenas de velas M1 de la apertura sin verificar horas despues de 3
	// reinicios). 1h es varias veces el hueco de un reinicio normal
	// (minutos) pero bien por debajo de una sesion completa.
	historyBatchDeepGapThreshold = time.Hour

	// unsubscribeDrainPeriod: dxLink no confirma cuando un FEED_SUBSCRIPTION
	// remove ya surtio efecto del lado del servidor (verificado contra la
	// especificacion oficial: no hay ACK, y el spec no dice nada sobre
	// eventos que ya estaban en camino). Mantener el dispatch registrado un
	// rato mas despues de mandar el remove absorbe esos rezagados en vez de
	// contarlos como huerfanos -- no cambia el resultado ya devuelto, solo
	// reduce el ruido de una condicion de carrera inherente al protocolo.
	unsubscribeDrainPeriod = 250 * time.Millisecond
)

// batchHistoryWait elige historyDeepWait para el lote entero si ALGUN
// simbolo necesita ponerse al dia con mas de historyBatchDeepGapThreshold --
// ver el comentario de esa constante arriba.
func batchHistoryWait(froms map[string]time.Time) time.Duration {
	now := time.Now()
	for _, from := range froms {
		if now.Sub(from) > historyBatchDeepGapThreshold {
			return historyDeepWait
		}
	}
	return historyDefaultWait
}

// FetchHistoryBatch pide el historial de un LOTE de simbolos en una sola
// suscripcion DxLink (cada uno con su propio FromTime) -- es el
// agrupamiento original del pool de Java (100 simbolos por canal) que el
// barrido nocturno usa para no pagar 13k round-trips de add/remove por
// timeframe. Mismo ciclo que FetchHistory: add del lote, rafaga, remove
// del lote, dispatch por simbolo para enrutar cada evento al collector.
func (p *CandlePool) FetchHistoryBatch(ctx context.Context, tf domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	if len(froms) == 0 {
		return map[string][]domain.Candle{}, nil
	}
	symbols := make([]string, 0, len(froms))
	for symbol := range froms {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	// Mismo lock por clave que FetchHistory, en orden lexicografico para
	// no interbloquearse con un FetchHistory por-simbolo concurrente.
	unlockAll := func() {
		for _, symbol := range symbols {
			if lockVal, ok := p.historyLocks.Load(candleKey(symbol, tf)); ok {
				lockVal.(*sync.Mutex).Unlock()
			}
		}
	}
	for _, symbol := range symbols {
		lockVal, _ := p.historyLocks.LoadOrStore(candleKey(symbol, tf), &sync.Mutex{})
		lockVal.(*sync.Mutex).Lock()
	}

	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		unlockAll()
		return nil, fmt.Errorf("allocating channel for %s history batch: %w", tf, err)
	}

	for _, symbol := range symbols {
		ch.occupy(candleKey(symbol, tf))
	}

	collector := newBatchHistoryCollector(tf)
	dispatchIDs := make(map[string]uint64, len(symbols))
	for _, symbol := range symbols {
		sym := symbol
		dispatchIDs[symbol] = p.registerDispatch(symbol, tf, "history-batch", func(ev rawCandleEvent) { collector.onCandle(sym, ev) })
	}

	cleanup := func() {
		for _, symbol := range symbols {
			ch.release(candleKey(symbol, tf))
		}
		_ = ch.channel.unsubscribeHistoryBatch(symbols, tf)
		time.AfterFunc(unsubscribeDrainPeriod, func() {
			for _, symbol := range symbols {
				p.unregisterDispatchIfCurrent(symbol, tf, dispatchIDs[symbol], "history-batch")
			}
		})
	}

	if err := ch.channel.subscribeHistoryBatch(symbols, tf, froms); err != nil {
		cleanup()
		unlockAll()
		return nil, fmt.Errorf("subscribing history batch: %w", err)
	}
	if err := waitForData(ctx, collector.settled, batchHistoryWait(froms)); err != nil {
		cleanup()
		unlockAll()
		return nil, err
	}
	result := collector.complete()
	cleanup()
	unlockAll()
	return result, nil
}

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
	if tf == domain.M1 && p.hasLiveSub(symbol) {
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
		return nil, err
	}
	result := collector.complete()
	cleanup()
	return result, nil
}
