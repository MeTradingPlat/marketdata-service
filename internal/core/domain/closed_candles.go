package domain

import "time"

// ClosedCandles devuelve solo las velas ya cerradas de un lote (backfill,
// ver ingestion) -- dxFeed manda OHLC parcial de la vela EN FORMACION igual
// que de las cerradas, asi que sin este filtro esa ultima vela a medio
// completar terminaria persistida como si fuera real.
//
// Dos señales, en orden de confianza: si el lote trae una vela POSTERIOR a
// esta, esa posterior ya prueba que esta cerro de verdad -- sin depender
// del reloj (mismo principio que usa handleLiveEvent para el M1 en vivo:
// "llego la vela siguiente" es una prueba real, "ya paso suficiente tiempo"
// es una suposicion). Solo la vela MAS RECIENTE del lote no tiene una
// posterior que la confirme -- ahi si se cae al reloj de pared como
// respaldo, unico caso donde este filtro dependia solo de comparar
// timestamp+duracion contra `now`.
func ClosedCandles(candles []Candle, now time.Time) []Candle {
	if len(candles) == 0 {
		return nil
	}
	var maxTs time.Time
	for _, c := range candles {
		if c.Timestamp.After(maxTs) {
			maxTs = c.Timestamp
		}
	}
	closed := make([]Candle, 0, len(candles))
	for _, c := range candles {
		if c.Timestamp.Before(maxTs) {
			closed = append(closed, c)
			continue
		}
		duration, err := c.Timeframe.Duration()
		if err == nil && !c.Timestamp.Add(duration).After(now) {
			closed = append(closed, c)
		}
	}
	return closed
}
