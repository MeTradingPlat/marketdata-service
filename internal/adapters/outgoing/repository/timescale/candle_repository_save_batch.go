package timescale

import (
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type batchResultExecer interface {
	Exec() (pgconn.CommandTag, error)
}

func upsertCandleQuery(tf domain.Timeframe) string {
	if candleTable(tf) == "candles_history" {
		return upsertCandleSQLCandlesHistory
	}
	return upsertCandleSQLCandles
}

func buildCandleSaveBatch(candles, latestDailies []domain.Candle, withWatermark bool) *pgx.Batch {
	batch := &pgx.Batch{}
	for _, c := range candles {
		batch.Queue(upsertCandleQuery(c.Timeframe), c.Symbol, string(c.Timeframe), c.Timestamp,
			c.Open, c.High, c.Low, c.Close, c.Volume, c.TradeCount, c.VWAP, c.Source, withWatermark)
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
