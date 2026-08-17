package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// m1GapDays: la ventana de verificacion de huecos -- la misma referencia
// de "10 dias" del prevClose (prevSessionCloseDays): dias de calendario
// bastan, los dias sin mercado no generan velas y no se marcan.
const m1GapDays = 10

// FillM1Gaps re-fetchea de TastyTrade los dias con huecos interiores de M1
// detectados por GetM1DayHoles (backstop del replay de la suscripcion en
// vivo, que cubre el hueco reciente; esto atrapa lo que el replay no
// alcanzo). El fetch pide el dia completo desde el inicio UTC y Save
// UPSERTea -- los minutos faltantes se rellenan y los existentes se
// conservan. Corre en el backfill, despues del rollout M1.
func FillM1Gaps(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, tracked []domain.Symbol) error {
	since := time.Now().UTC().AddDate(0, 0, -m1GapDays)
	holes, err := candles.GetM1DayHoles(ctx, since)
	if err != nil {
		return fmt.Errorf("detecting M1 day holes: %w", err)
	}
	if len(holes) == 0 {
		log.Info().Msg("M1 gap check: no holes found in the last days")
		return nil
	}

	start := time.Now()
	filled := 0
	for _, hole := range holes {
		fetched, err := gateway.GetCandles(ctx, hole.Symbol, domain.M1, hole.Day)
		if err != nil || len(fetched) == 0 {
			continue
		}
		closed := domain.ClosedCandles(fetched, time.Now())
		if len(closed) == 0 {
			continue
		}
		if err := candles.Save(ctx, closed); err != nil {
			log.Error().Err(err).Str("symbol", hole.Symbol).Time("day", hole.Day).Msg("M1 gap fill save failed, leaving for next cycle")
			continue
		}
		filled++
	}
	log.Info().Int("holes", len(holes)).Int("filled", filled).Dur("elapsed", time.Since(start)).Msg("M1 gap fill finished")
	return nil
}
