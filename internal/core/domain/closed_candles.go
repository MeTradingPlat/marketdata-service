package domain

import "time"

// ClosedCandles devuelve solo las velas cuyo periodo ya termino a la hora
// dada -- el mismo filtro que usa el backfill (ver ingestion) para no
// persistir la vela EN FORMACION cuando se guarda un lote traido de un
// probe directo (dxFeed manda OHLC parcial de la vela en formacion igual
// que de las cerradas).
func ClosedCandles(candles []Candle, now time.Time) []Candle {
	closed := make([]Candle, 0, len(candles))
	for _, c := range candles {
		duration, err := c.Timeframe.Duration()
		if err == nil && !c.Timestamp.Add(duration).After(now) {
			closed = append(closed, c)
		}
	}
	return closed
}
