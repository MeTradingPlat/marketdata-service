package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// RefreshPrevPostMarketVolume calcula el postmarket de la sesion anterior
// (16:00 ET-medianoche del ultimo dia habil) SOLO para los simbolos cuyo
// prev_post_market_volume_updated_at quedo fuera de la ventana de
// mantenimiento actual -- mismo guard y mismo motivo que RefreshPrevClose.
// Sin este dato, un escaner que corre en premarket nunca ve un
// postMarketVolume real para TIPO_VOLUMEN=AMBOS (ver domain.Fundamentals.
// PrevPostMarketVolume). Lectura y escritura son cada una UN SOLO round
// trip para todo el lote stale.
func RefreshPrevPostMarketVolume(ctx context.Context, candles out.CandleRepository, fundamentals out.FundamentalsRepository, windowStart time.Time) error {
	stale, err := fundamentals.GetSymbolsWithStalePrevPostMarketVolume(ctx, windowStart)
	if err != nil {
		return fmt.Errorf("listing symbols with stale prev post market volume: %w", err)
	}
	if len(stale) == 0 {
		log.Info().Msg("prev post market volume refresh: all symbols already done for this maintenance window, skipping")
		return nil
	}

	start := time.Now()
	volumes, err := candles.GetPreviousPostMarketVolumeBatch(ctx, stale, time.Now())
	if err != nil {
		return fmt.Errorf("loading previous post market volume batch: %w", err)
	}

	attemptedOnly := make([]string, 0, len(stale))
	for _, symbol := range stale {
		if _, ok := volumes[symbol]; !ok {
			attemptedOnly = append(attemptedOnly, symbol)
		}
	}

	if err := fundamentals.UpsertPrevPostMarketVolumeBatch(ctx, volumes, attemptedOnly); err != nil {
		return fmt.Errorf("upserting prev post market volume batch: %w", err)
	}

	log.Info().Int("found", len(volumes)).Int("attempted_only", len(attemptedOnly)).Dur("elapsed", time.Since(start)).Msg("prev post market volume refresh finished")
	return nil
}
