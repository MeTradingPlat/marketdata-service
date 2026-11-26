package tastytrade

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// handleLiveEvent distingue actualizacion de la vela en formacion, cierre organico y correccion tardia de una vela ya cerrada (via dispatchClosed, fusionada contra lastClosed).
func (p *CandlePool) handleLiveEvent(symbol string, ev rawCandleEvent) {
	now := time.Now()
	p.lastLiveEventAtUnixNano.Store(now.UnixNano())
	p.noteLiveEvent(symbol, now)
	p.noteNewBar(symbol, ev.Timestamp, now)
	p.currentMu.Lock()
	prev, exists := p.current[symbol]
	last, hasLast := p.lastClosed[symbol]
	// isLateCorrection cubre los dos caminos por los que un minuto puede
	// haber quedado "ya cerrado" antes de que este evento llegara: el cierre
	// organico de arriba (existe un "current" mas nuevo) y el cierre por
	// tiempo de CloseElapsedForming (current ya se borro porque nadie mando
	// un tick a tiempo, pero lastClosed s? tiene ese minuto). En los dos
	// casos hay que fusionar contra lastClosed, no arrancar una vela "nueva"
	// para un minuto que el sistema ya dio por cerrado y publico.
	if isLateCandleEvent(ev, prev, last, exists, hasLast) {
		p.handleLateCorrection(symbol, ev, last, hasLast)
		return
	}

	var forming domain.Candle
	if exists && !prev.Timestamp.Equal(ev.Timestamp) {
		// Cierre organico: llego un tick de un minuto mas nuevo; no se sintetizan velas planas para los minutos intermedios.
		closed := prev
		p.current[symbol] = mergeCandle(domain.Candle{}, ev, symbol, domain.M1)
		p.lastClosed[symbol] = closed
		forming = p.current[symbol]
		p.currentMu.Unlock()
		p.dispatchClosed(symbol, closed)
		p.dispatchTick(symbol, forming)
		return
	}
	forming = mergeCandle(prev, ev, symbol, domain.M1)
	p.current[symbol] = forming
	p.currentMu.Unlock()
	p.dispatchTick(symbol, forming)
}

// closeElapsedGracePeriod: margen sobre el fin exacto del minuto antes de
// forzar el cierre por tiempo -- deja que un tick que ya esta en camino (la
// latencia normal de red/procesamiento) todavia alcance a cerrar la vela
// por la via organica de arriba antes de que el sweep la fuerce con los
// datos que tenia hasta ese momento. Mismo margen que ya usa
// signal-processing-service del otro lado (_BAR_CLOSE_BUFFER_SECONDS) y que
// RangeAggregated (RecentCache) para el mismo motivo.
const closeElapsedGracePeriod = 1 * time.Second

// CloseElapsedForming cierra por RELOJ cualquier vela M1 en formacion cuyo
// minuto ya termino, sin esperar un tick del minuto siguiente -- sin esto,
// un simbolo que se queda sin ticks justo en el cierre (silencio real de
// mercado, no una suscripcion muerta) deja esa vela "en formacion" para
// siempre hasta el proximo trade, que en un simbolo poco liquido puede
// tardar minutos u horas (confirmado como el patron esperado: cierre por
// tiempo de pared, no por llegada del proximo dato). Se llama
// periodicamente desde un ticker de 1s (ver cmd/api) -- barata: solo itera
// el mapa de velas en formacion, sin ningun I/O de red de por medio.
func (p *CandlePool) CloseElapsedForming(now time.Time) {
	type closedEntry struct {
		symbol string
		candle domain.Candle
	}
	var toClose []closedEntry

	p.currentMu.Lock()
	for symbol, c := range p.current {
		if !now.Before(c.Timestamp.Add(time.Minute + closeElapsedGracePeriod)) {
			toClose = append(toClose, closedEntry{symbol: symbol, candle: c})
			delete(p.current, symbol)
			p.lastClosed[symbol] = c
		}
	}
	p.currentMu.Unlock()

	for _, entry := range toClose {
		p.dispatchClosed(entry.symbol, entry.candle)
	}
}

// flushFormingCandles guarda la vela en formacion de cada simbolo antes de que StopAllLive/CloseAllConnections la descarten.
func (p *CandlePool) flushFormingCandles() {
	p.currentMu.Lock()
	pending := make(map[string]domain.Candle, len(p.current))
	for symbol, c := range p.current {
		pending[symbol] = c
	}
	p.currentMu.Unlock()

	for symbol, c := range pending {
		p.dispatchClosed(symbol, c)
	}
}

func (p *CandlePool) dispatchClosed(symbol string, c domain.Candle) {
	if !c.IsComplete() {
		return
	}
	p.liveMu.Lock()
	cb := p.liveSubs[symbol]
	p.liveMu.Unlock()
	if cb != nil {
		cb(c)
	}
}

// dispatchTick reenvia la vela en formacion tras cada tick -- el suscriptor
// (StreamLive -> Broadcaster) descarta si su canal va lleno, asi un cliente
// lento jamas frena el merge de ticks de la conexion DxLink. El filtro es
// SOLO "tiene cierre": un tick parcial (minuto sin trades, primer evento
// del periodo con OHLC incompleto) se dibuja plano en el grafico y el
// siguiente tick lo completa -- descartarlo por IsComplete dejaba huecos en
// la serie en vivo (minutos enteros saltados).
func (p *CandlePool) dispatchTick(symbol string, c domain.Candle) {
	if c.Close == 0 {
		return
	}
	p.liveMu.Lock()
	cb := p.liveTicks[symbol]
	p.liveMu.Unlock()
	if cb != nil {
		cb(c)
	}
}
