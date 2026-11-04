package timescale

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func candleTable(tf domain.Timeframe) string {
	switch tf {
	case domain.D1, domain.H1:
		return "candles_history"
	default:
		return "candles"
	}
}
