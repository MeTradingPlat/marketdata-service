package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// intradaySessionsSQL agrega las M1 de hoy en las 3 sesiones en una sola pasada; los limites se calculan en Go para usar el indice (symbol_id, timeframe, ts).
const intradaySessionsSQL = `
	SELECT
		(ARRAY_AGG(c.open ORDER BY c.ts) FILTER (WHERE c.ts >= $4 AND c.ts < $5))[1],
		MAX(c.high) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
		MIN(c.low) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
		COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $4 AND c.ts < $5), 0),
		COALESCE(SUM(c.volume) FILTER (WHERE c.ts < $4), 0),
		(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts < $4))[1],
		COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $5), 0),
		(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts >= $5))[1]
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE s.symbol = $1 AND c.timeframe = 'M1' AND c.ts >= $2 AND c.ts < $3
`

func (r *CandleRepository) GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("loading America/New_York location: %w", err)
	}
	nowET := time.Now().In(loc)
	dayStart := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	marketOpen := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 16, 0, 0, 0, loc)

	snap := domain.IntradaySnapshot{Symbol: symbol}
	var open, high, low, preClose, postClose *float64
	err = r.pool.QueryRow(ctx, intradaySessionsSQL, symbol, dayStart, dayEnd, marketOpen, marketClose).Scan(
		&open, &high, &low, &snap.DayVolume,
		&snap.PreMarketVolume, &preClose,
		&snap.PostMarketVolume, &postClose,
	)
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("querying intraday sessions for %s: %w", symbol, err)
	}
	if open != nil {
		snap.Open = *open
	}
	if high != nil {
		snap.High = *high
	}
	if low != nil {
		snap.Low = *low
	}
	if preClose != nil {
		snap.PreMarketClose = *preClose
	}
	if postClose != nil {
		snap.PostMarketClose = *postClose
	}
	return snap, nil
}

// GetIntradaySessionsBatch es intradaySessionsSQL con GROUP BY s.symbol.
// universeBatchChunkSize acota cuantos simbolos entran en un solo ANY($1) para limitar el pico de memoria por consulta.
const universeBatchChunkSize = 1500

func (r *CandleRepository) GetIntradaySessionsBatch(ctx context.Context, symbols []string) (map[string]domain.IntradaySnapshot, error) {
	result := make(map[string]domain.IntradaySnapshot, len(symbols))
	if len(symbols) == 0 {
		return result, nil
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading America/New_York location: %w", err)
	}
	nowET := time.Now().In(loc)
	dayStart := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	marketOpen := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 16, 0, 0, 0, loc)

	for _, batch := range chunkSymbols(symbols, universeBatchChunkSize) {
		if err := r.queryIntradaySessionsBatch(ctx, batch, dayStart, dayEnd, marketOpen, marketClose, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *CandleRepository) queryIntradaySessionsBatch(ctx context.Context, symbols []string, dayStart, dayEnd, marketOpen, marketClose time.Time, result map[string]domain.IntradaySnapshot) error {
	query := `
		SELECT
			s.symbol,
			(ARRAY_AGG(c.open ORDER BY c.ts) FILTER (WHERE c.ts >= $4 AND c.ts < $5))[1],
			MAX(c.high) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
			MIN(c.low) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
			COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $4 AND c.ts < $5), 0),
			COALESCE(SUM(c.volume) FILTER (WHERE c.ts < $4), 0),
			(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts < $4))[1],
			COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $5), 0),
			(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts >= $5))[1]
		FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
		WHERE s.symbol = ANY($1) AND c.timeframe = 'M1' AND c.ts >= $2 AND c.ts < $3
		GROUP BY s.symbol
	`
	rows, err := r.snapshotPool.Query(ctx, query, symbols, dayStart, dayEnd, marketOpen, marketClose)
	if err != nil {
		return fmt.Errorf("querying intraday sessions batch for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()

	for rows.Next() {
		var snap domain.IntradaySnapshot
		var open, high, low, preClose, postClose *float64
		if err := rows.Scan(&snap.Symbol, &open, &high, &low, &snap.DayVolume,
			&snap.PreMarketVolume, &preClose, &snap.PostMarketVolume, &postClose); err != nil {
			return fmt.Errorf("scanning intraday sessions batch row: %w", err)
		}
		if open != nil {
			snap.Open = *open
		}
		if high != nil {
			snap.High = *high
		}
		if low != nil {
			snap.Low = *low
		}
		if preClose != nil {
			snap.PreMarketClose = *preClose
		}
		if postClose != nil {
			snap.PostMarketClose = *postClose
		}
		result[snap.Symbol] = snap
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating intraday sessions batch rows: %w", err)
	}
	return nil
}
