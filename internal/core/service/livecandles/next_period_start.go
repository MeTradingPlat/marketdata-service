package livecandles

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func NextPeriodStart(start time.Time, tf domain.Timeframe) time.Time {
	switch tf {
	case domain.W1:
		return start.AddDate(0, 0, 7)
	case domain.MO1:
		return start.AddDate(0, 1, 0)
	case domain.MO3:
		return start.AddDate(0, 3, 0)
	case domain.MO6:
		return start.AddDate(0, 6, 0)
	case domain.Y1:
		return start.AddDate(1, 0, 0)
	}
	if d, err := tf.Duration(); err == nil {
		return start.Add(d)
	}
	if _, _, approx, ok := tf.Aggregation(); ok && approx > 0 {
		return start.Add(approx)
	}
	return start.Add(time.Minute)
}
