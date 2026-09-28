package metadata

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

func withRealSessionVolumes(tracker *intraday.DayVolumeTracker, symbol string, snapshot domain.IntradaySnapshot, now time.Time) domain.IntradaySnapshot {
	current, known := tracker.Get(symbol)
	if !known {
		return snapshot
	}
	phase := intraday.PhaseAt(now)
	if phase == intraday.PhasePreMarket {
		snapshot.PreMarketVolume = current
		return snapshot
	}
	// Extended (TradeETH.dayVolumeAsDouble en vivo) reemplaza la foto puntual
	// de preMarketEnd cuando ya esta disponible -- no se mueve durante sesion
	// regular, asi que sirve igual sin depender de haberla capturado en la
	// ventana exacta 9:26-9:28. preMarketEnd sigue de respaldo para un
	// simbolo que arranco a operar recien y todavia no tiene Extended.
	if phase != intraday.PhaseClosed {
		if extended, ok := tracker.Extended(symbol); ok {
			snapshot.PreMarketVolume = extended
		} else if preMarket, ok := tracker.Boundary(symbol, intraday.PreMarketEnd); ok {
			snapshot.PreMarketVolume = preMarket
		}
	}
	if regularEnd, ok := tracker.Boundary(symbol, intraday.RegularEnd); ok && phase == intraday.PhasePostMarket {
		snapshot.PostMarketVolume = max(current-regularEnd, 0)
	}
	return snapshot
}
