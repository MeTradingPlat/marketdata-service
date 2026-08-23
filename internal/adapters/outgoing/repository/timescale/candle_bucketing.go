package timescale

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// aggregateIntoBuckets agrupa velas crudas (orden DESC por ts) en buckets de
// ancho fijo bucketWidth sin pasar por GROUP BY de Postgres -- confirmado en
// vivo el 2026-08-23: la misma agregacion via SQL (ARRAY_AGG+GroupAggregate+
// Sort) tardaba 1064ms para un lote de 700 simbolos contra 75ms de leer esas
// mismas filas crudas sin agrupar (14x). time.Time.Truncate() en UTC alinea
// igual que time_bucket() de Postgres para anchos que dividen el dia exacto
// (minutos/horas -- verificado en vivo contra 5min/3min/2h/12h) -- por eso
// solo se usa para timeframes derivados de M1/H1 (ver sourcePeriodOf), NUNCA
// para semana/mes/anio calendario (esa alineacion no es de ancho fijo).
// Devuelve hasta maxBuckets buckets, en el mismo orden DESC de entrada.
func aggregateIntoBuckets(raw []domain.Candle, timeframe domain.Timeframe, bucketWidth time.Duration, maxBuckets int) []domain.Candle {
	result := make([]domain.Candle, 0, maxBuckets)
	var acc domain.Candle
	var bucketStart time.Time
	open := false

	flushIfOpen := func() bool {
		if open {
			result = append(result, acc)
		}
		return len(result) >= maxBuckets
	}

	for _, c := range raw {
		b := c.Timestamp.UTC().Truncate(bucketWidth)
		if !open || !b.Equal(bucketStart) {
			if flushIfOpen() {
				return result
			}
			bucketStart = b
			acc = domain.Candle{
				Symbol: c.Symbol, Timeframe: timeframe, Source: "aggregated",
				Timestamp: b, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close,
				Volume: c.Volume, TradeCount: c.TradeCount,
			}
			open = true
			continue
		}
		// c es mas vieja que la anterior (entrada DESC) -- cada nueva fila del
		// mismo bucket es el open real hasta que el bucket cambie.
		acc.Open = c.Open
		if c.High > acc.High {
			acc.High = c.High
		}
		if c.Low < acc.Low {
			acc.Low = c.Low
		}
		acc.Volume += c.Volume
		acc.TradeCount += c.TradeCount
	}
	flushIfOpen()
	return result
}

// sourcePeriodOf devuelve la duracion de una vela del timeframe base -- solo
// M1/H1 usan aggregateIntoBuckets; D1 como fuente (D2/D3/W1/MO1/MO3/MO6/Y1)
// sigue agregandose en SQL porque sus buckets son de calendario (semana/mes/
// anio), no de ancho fijo, y el volumen de filas D1 por simbolo es chico de
// todas formas -- no hay beneficio real que justifique replicar ese
// alineamiento calendario en Go.
func sourcePeriodOf(source domain.Timeframe) (time.Duration, bool) {
	switch source {
	case domain.M1:
		return time.Minute, true
	case domain.H1:
		return time.Hour, true
	default:
		return 0, false
	}
}
