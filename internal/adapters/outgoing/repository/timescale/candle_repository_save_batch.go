package timescale

import (
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type batchResultExecer interface {
	Exec() (pgconn.CommandTag, error)
}

type batchResultCloser interface {
	batchResultExecer
	Close() error
}

func consumeCandleBatchResults(results batchResultCloser, candleCount, dailyCount int, withWatermark bool) (int64, error) {
	affected, err := readCandleBatchResults(results, candleCount, dailyCount, withWatermark)
	closeErr := results.Close()
	if err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, fmt.Errorf("closing candle batch results: %w", closeErr)
	}
	return affected, nil
}

func upsertCandleQuery(tf domain.Timeframe) string {
	if candleTable(tf) == "candles_history" {
		return upsertCandleSQLCandlesHistory
	}
	return upsertCandleSQLCandles
}

func observedAtParam(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func buildCandleSaveBatch(candles, latestDailies []domain.Candle, withWatermark bool) *pgx.Batch {
	batch := &pgx.Batch{}
	for _, c := range candles {
		batch.Queue(upsertCandleQuery(c.Timeframe), c.Symbol, string(c.Timeframe), c.Timestamp,
			c.Open, c.High, c.Low, c.Close, c.Volume, c.TradeCount, c.VWAP, c.Source, withWatermark, observedAtParam(c.ObservedAt))
		if withWatermark {
			batch.Queue(upsertWatermarkSQL, c.Symbol, string(c.Timeframe), c.Timestamp)
		}
	}
	for _, c := range latestDailies {
		batch.Queue(updateLastVolumeSQL, c.Symbol, c.Volume, c.Timestamp)
	}
	return batch
}

func readCandleBatchResults(results batchResultExecer, candleCount, dailyCount int, withWatermark bool) (int64, error) {
	var affected int64
	for range candleCount {
		tag, err := results.Exec()
		if err != nil {
			return 0, fmt.Errorf("upserting candle batch: %w", err)
		}
		affected += tag.RowsAffected()
		if withWatermark {
			if _, err := results.Exec(); err != nil {
				return 0, fmt.Errorf("upserting watermark batch: %w", err)
			}
		}
	}
	return affected, readDailyVolumeResults(results, dailyCount)
}

func readDailyVolumeResults(results batchResultExecer, dailyCount int) error {
	for range dailyCount {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("updating last volume: %w", err)
		}
	}
	return nil
}
