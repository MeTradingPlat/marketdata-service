package tastytrade

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func isLateCandleEvent(ev rawCandleEvent, prev, last domain.Candle, exists, hasLast bool) bool {
	if exists {
		return ev.Timestamp.Before(prev.Timestamp)
	}
	return hasLast && !ev.Timestamp.After(last.Timestamp)
}

func (p *CandlePool) handleLateCorrection(symbol string, ev rawCandleEvent, last domain.Candle, hasLast bool) {
	base := domain.Candle{}
	if hasLast && last.Timestamp.Equal(ev.Timestamp) {
		base = last
	}
	corrected := mergeCandle(base, ev, symbol, domain.M1)
	unchanged := candleValuesEqual(corrected, base)
	p.lastClosed[symbol] = corrected
	p.currentMu.Unlock()
	// unchanged descarta el replay repetido de historial reciente (la retomada
	// tras reconectar reenvia velas ya cerradas para todo simbolo en vivo, no
	// solo los que tuvieron una correccion) -- sin este filtro, cada retomada
	// reencolaba en liveSaveBuffer las mismas velas ya guardadas de los
	// ~13k simbolos en vivo, multiplicando el tamaño del lote de guardado
	// sin ningun dato nuevo real.
	if !unchanged {
		p.dispatchClosed(symbol, corrected)
		return
	}
	p.lateUnchangedDiscards.record(symbol, time.Now(), "discarded unchanged late candle correction")
}
