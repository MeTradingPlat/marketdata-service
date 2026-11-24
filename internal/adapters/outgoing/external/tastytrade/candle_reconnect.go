package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// handleConnectionReconnect corre cuando UNA conexion del pool se
// reconecta -- sus canales viejos ya no sirven (IDs de un socket que ya no
// existe), pero la conexion en si sigue siendo la misma valida, asi que se
// resetea (no se saca del pool) y se vuelve a pedir un slot para cada
// simbolo en vivo que tenia.
func (p *CandlePool) handleConnectionReconnect(ctx context.Context, pc *pooledConnection) {
	pc.resubMu.Lock()
	defer pc.resubMu.Unlock()

	symbols := pc.takeLiveSymbols()

	// La vela a medio formar de cada simbolo queda incompleta -- le faltan
	// los ticks de los segundos que la conexion estuvo caida -- asi que se
	// descarta de current, pero su timestamp se guarda como punto de
	// retomada: resuscribir con FromTime = ese timestamp le pide a dxLink
	// que repita desde ahi (la vela incompleta incluida, esta vez completa)
	// en vez de dejar el hueco perdido para siempre.
	p.currentMu.Lock()
	resumeFrom := make(map[string]time.Time, len(symbols))
	for _, symbol := range symbols {
		if prev, ok := p.current[symbol]; ok && !prev.Timestamp.IsZero() {
			resumeFrom[symbol] = prev.Timestamp
		} else if last, ok := p.lastClosed[symbol]; ok && !last.Timestamp.IsZero() {
			resumeFrom[symbol] = last.Timestamp
		}
		delete(p.current, symbol)
	}
	p.currentMu.Unlock()

	now := time.Now()
	p.activityMu.Lock()
	p.ensureActivityMaps()
	for _, symbol := range symbols {
		if last, ok := p.lastEventAt[symbol]; ok && now.Sub(last) <= 10*time.Minute {
			p.pendingRevive[symbol] = reviveState{reconnectedAt: now}
		}
	}
	p.activityMu.Unlock()

	retriesLeft := resubscribeRetryBudget
	for _, symbol := range symbols {
		p.liveMu.Lock()
		cb := p.liveSubs[symbol]
		tick := p.liveTicks[symbol]
		p.liveMu.Unlock()
		if cb == nil {
			continue
		}
		subscribe := func() error { return p.SubscribeLive(ctx, symbol, resumeFrom[symbol], cb, tick) }
		if err := resubscribeWithRetry(ctx, subscribe, &retriesLeft, resubscribeRetryDelay); err != nil {
			// La entrada en liveSubs es la memoria de que el simbolo debe estar
			// vivo y ReattachOrphans lo reengancha; borrarla dejaba al
			// reconciliador (100 simbolos/5 min) como unico camino.
			p.activityMu.Lock()
			delete(p.pendingRevive, symbol)
			p.activityMu.Unlock()
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to resubscribe live candle after reconnect, leaving it for orphan heal")
		}
	}
}

// ForceReconnectAll cierra y reconecta CADA conexion del pool, sin importar
// si el socket se ve sano -- la unica forma confirmada de recuperar un
// silencio de datos donde el KEEPALIVE seguia respondiendo con normalidad
// (ver live_data_watchdog.go). Reusa el mismo camino de cleanup+reconexion
// que ya usa healthCheckLoop por conexion individual, asi que cada una
// vuelve a autenticar y a resuscribir sus simbolos en vivo por su cuenta
// (ver handleConnectionReconnect) -- no hace falta reimplementar nada de
// eso aca.
func (p *CandlePool) ForceReconnectAll(ctx context.Context) {
	p.allocator.forceReconnectAll(ctx)
}
