package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	narrowSeriesWindow  = 24 * time.Hour
	maxNarrowSeriesBars = 60
)

type seriesQuery func(ctx context.Context, symbols []string, window time.Duration) (map[string][]domain.Candle, error)

func getSeriesFrom(ctx context.Context, pool *pgxpool.Pool, symbols []string, timeframe domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	if len(symbols) == 0 {
		return map[string][]domain.Candle{}, nil
	}
	duration, err := timeframe.Duration()
	if err != nil {
		return nil, fmt.Errorf("resolving duration for %s: %w", timeframe, err)
	}
	now := time.Now()
	query := func(ctx context.Context, subset []string, window time.Duration) (map[string][]domain.Candle, error) {
		return getSeriesSince(ctx, pool, subset, timeframe, bars, now.Add(-window))
	}
	return getSeriesTiered(ctx, symbols, bars, seriesLookbackTiers(bars, duration), query)
}

func seriesLookbackTiers(bars int, duration time.Duration) []time.Duration {
	full := seriesLookbackWindow(bars, duration)
	if duration >= 24*time.Hour || bars > maxNarrowSeriesBars || full <= narrowSeriesWindow {
		return []time.Duration{full}
	}
	return []time.Duration{narrowSeriesWindow, full}
}

func getSeriesTiered(ctx context.Context, symbols []string, bars int, tiers []time.Duration, query seriesQuery) (map[string][]domain.Candle, error) {
	result := make(map[string][]domain.Candle, len(symbols))
	pending := symbols
	for _, window := range tiers {
		found, err := query(ctx, pending, window)
		if err != nil {
			return nil, err
		}
		var unsatisfied []string
		for _, symbol := range pending {
			series, ok := found[symbol]
			if ok {
				result[symbol] = series
			}
			if len(series) < bars {
				unsatisfied = append(unsatisfied, symbol)
			}
		}
		if len(unsatisfied) == 0 {
			break
		}
		pending = unsatisfied
	}
	return result, nil
}
