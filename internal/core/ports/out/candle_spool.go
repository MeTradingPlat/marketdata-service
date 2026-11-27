package out

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

type CandleSpool interface {
	Write(candles []domain.Candle) error
	Pending() ([]string, error)
	Read(id string) ([]domain.Candle, error)
	Remove(id string) error
}
