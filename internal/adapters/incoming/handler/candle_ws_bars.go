package handler

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

// seedAggregate arranca el agregado compartido (candleAggregateHub) del
// primer suscriptor de un (symbol, timeframe) con la vela en formacion que
// ya existia -- sin esto, el hub creado recien perderia el progreso que
// GetCurrentCandle ya tenia hasta que llegue el proximo tick real.
func seedAggregate(seed *dto.CandleBar, tf domain.Timeframe, now time.Time) *dto.CandleBar {
	if seed == nil {
		return nil
	}
	start := *seed
	start.Time = livecandles.FormingPeriodStart(now, tf).Unix()
	return &start
}

func toBars(candles []domain.Candle) []dto.CandleBar {
	bars := make([]dto.CandleBar, len(candles))
	for i, c := range candles {
		bars[i] = toBar(c)
	}
	return bars
}

func toBar(c domain.Candle) dto.CandleBar {
	return dto.CandleBar{
		Time:   c.Timestamp.Unix(),
		Open:   c.Open,
		High:   c.High,
		Low:    c.Low,
		Close:  c.Close,
		Volume: c.Volume,
		Closed: true,
	}
}
