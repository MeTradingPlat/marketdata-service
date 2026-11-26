package ingestion

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func sampleCandleSymbols(candles []domain.Candle, limit int) []string {
	seen := make(map[string]struct{}, min(len(candles), limit))
	sample := make([]string, 0, min(len(candles), limit))
	for _, c := range candles {
		if _, ok := seen[c.Symbol]; ok {
			continue
		}
		seen[c.Symbol] = struct{}{}
		sample = append(sample, c.Symbol)
		if len(sample) >= limit {
			break
		}
	}
	return sample
}

func reportRetryOverflow(dropped int, total int64, sample []string, notify func(level, message string, metadata map[string]any)) {
	log.Error().
		Int("discarded", dropped).
		Int64("totalDiscarded", total).
		Strs("symbols", sample).
		Msg("save retry buffer overflow, discarding candles")
	if notify != nil {
		notify("ERROR", "Buffer de reintento de velas supero 100k y descarto velas", map[string]any{
			"descartadasLote":  dropped,
			"totalDescartadas": total,
			"muestraSimbolos":  sample,
		})
	}
}
