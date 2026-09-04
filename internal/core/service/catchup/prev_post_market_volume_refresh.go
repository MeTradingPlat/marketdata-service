package catchup

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// RefreshPrevPostMarketVolume calcula el postmarket de la sesion anterior
// (16:00 ET-medianoche del ultimo dia habil) SOLO para los simbolos cuyo
// prev_post_market_volume_updated_at quedo fuera de la ventana de
// mantenimiento actual -- mismo guard y mismo motivo que RefreshPrevClose
// (evita recalcular el universo entero en cada reinicio). Sin este dato, un
// escaner que corre en premarket nunca ve un postMarketVolume real para
// TIPO_VOLUMEN=AMBOS (ver domain.Fundamentals.PrevPostMarketVolume).
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

	var done, failed atomic.Int64

	jobs := make(chan string, len(stale))
	for _, symbol := range stale {
		jobs <- symbol
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < prevCloseWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				if writeOnePrevPostMarketVolume(ctx, fundamentals, symbol, volumes) {
					done.Add(1)
				} else {
					failed.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	log.Info().Int64("symbols", done.Load()).Int64("failed", failed.Load()).Dur("elapsed", time.Since(start)).Msg("prev post market volume refresh finished")
	return nil
}

// writeOnePrevPostMarketVolume: mismo criterio que writeOnePrevClose -- un
// simbolo fallido no frena a los demas, el proximo window lo reintenta.
func writeOnePrevPostMarketVolume(ctx context.Context, fundamentals out.FundamentalsRepository, symbol string, volumes map[string]int64) bool {
	volume, ok := volumes[symbol]
	if !ok {
		return fundamentals.MarkPrevPostMarketVolumeAttempted(ctx, symbol) == nil
	}
	return fundamentals.UpsertPrevPostMarketVolume(ctx, symbol, volume) == nil
}
