package timescale

import (
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

var candleUpsertShortBatches atomic.Int64

func CandleUpsertShortBatches() int64 {
	return candleUpsertShortBatches.Load()
}

func checkBatchRowsAffected(expected, affected int64, withWatermark bool) bool {
	if !withWatermark || affected >= expected {
		return false
	}
	candleUpsertShortBatches.Add(1)
	log.Warn().
		Int64("expected", expected).
		Int64("affected", affected).
		Bool("withWatermark", withWatermark).
		Msg("candle upsert affected fewer rows than expected")
	return true
}
