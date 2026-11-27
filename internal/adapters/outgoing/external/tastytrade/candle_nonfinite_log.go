package tastytrade

import (
	"sync"

	"github.com/rs/zerolog/log"
)

var nonFiniteCandleRecordOnce sync.Once

func isNonFiniteString(s string) bool {
	return s == "NaN" || s == "Infinity" || s == "-Infinity"
}

func hasNonFiniteString(record []interface{}) bool {
	for _, v := range record {
		if s, ok := v.(string); ok && isNonFiniteString(s) {
			return true
		}
	}
	return false
}

func logNonFiniteRecordOnce(record []interface{}) {
	if !hasNonFiniteString(record) {
		return
	}
	nonFiniteCandleRecordOnce.Do(func() {
		log.Info().Interface("record", record).Msg("dxlink candle record carries a non-finite string value")
	})
}
