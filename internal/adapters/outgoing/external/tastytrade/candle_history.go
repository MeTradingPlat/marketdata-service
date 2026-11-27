package tastytrade

import (
	"context"
	"fmt"
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
	symbols := p.batchTargetSymbols(tf, froms)
	if len(symbols) == 0 {
		return map[string][]domain.Candle{}, nil
	}
	res, err := p.historyLimiter.acquire(ctx, len(symbols))
	if err != nil {
		return nil, err
	}
	defer res.release()
	unlockAll := p.lockHistorySymbols(symbols, tf)
	defer unlockAll()

	collector := newBatchHistoryCollector(tf)
	active, dispatchIDs := p.registerBatchDispatch(symbols, tf, collector)
	if len(active) == 0 {
		return map[string][]domain.Candle{}, nil
	}
	ch, err := p.allocator.allocate(ctx, candleKeys(active, tf)...)
	if err != nil {
		p.unregisterBatchDispatch(active, tf, dispatchIDs)
		return nil, fmt.Errorf("allocating channel for %s history batch: %w", tf, err)
	}
	return p.fetchBatchOnChannel(ctx, ch, active, tf, froms, collector, dispatchIDs, res)
}
