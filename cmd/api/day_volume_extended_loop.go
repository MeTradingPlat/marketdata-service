package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/rs/zerolog/log"
)

// dayVolumeExtendedInterval: LiveExtendedDayVolumes es una lectura pura en
// memoria (ver day_volume_live.go), sin ningun costo de red -- un intervalo
// corto no tiene el motivo de "no saturar DxLink" que si aplica a
// dayVolumeRefreshInterval (ese si dispara un fetch real).
const dayVolumeExtendedInterval = 30 * time.Second

// StartDayVolumeExtendedLoop mantiene DayVolumeTracker.Extended al dia con
// TradeETH.dayVolumeAsDouble -- reemplaza la foto puntual de PreMarketEnd
// (capturada una sola vez en la ventana fragil 9:26-9:28, ver
// day_volume_boundary_loop.go) por un valor que se actualiza solo y no
// necesita capturarse en un segundo exacto: TradeETH no se mueve durante la
// sesion regular, asi que leerlo en cualquier momento entre las 9:30 y las
// 16:00 da el mismo resultado. PreMarketEnd se deja como respaldo (ver
// withRealSessionVolumes) para un simbolo que todavia no tiene Extended.
func StartDayVolumeExtendedLoop(ctx context.Context, gateway out.DayVolumeGateway, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	go func() {
		ticker := time.NewTicker(dayVolumeExtendedInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !isDayVolumeRefreshWindow(time.Now()) {
					continue
				}
				refreshExtendedDayVolumes(ctx, gateway, symbols, tracker)
			}
		}
	}()
}

func refreshExtendedDayVolumes(ctx context.Context, gateway out.DayVolumeGateway, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	syms, day, err := trackedSymbolsAndDayET(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("extended day volume refresh: failed to list tracked symbols")
		return
	}
	volumes := gateway.LiveExtendedDayVolumes(syms)
	tracker.SetExtended(day, volumes)
	// Mismo truco para post-market: seguir pisando el baseline mientras no
	// es post-market (TradeETH no se mueve en regular, da igual cual sea
	// "la ultima" escritura) y dejar de llamarlo apenas empieza -- el ultimo
	// valor escrito queda congelado solo, sin ventana de segundos que
	// acertar (ver withRealSessionVolumes).
	if intraday.PhaseAt(time.Now()) != intraday.PhasePostMarket {
		tracker.SetExtendedBaseline(day, volumes)
	}
}
