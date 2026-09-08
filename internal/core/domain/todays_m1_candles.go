package domain

import "time"

// TodaysM1Candles filtra un lote a solo las velas M1 del dia de mercado
// (ET) de `now` -- pensado para alimentar SnapshotTracker.RecordClosedCandle
// desde un camino de guardado en lote (backfill/catchup) que no pasa por el
// stream en vivo. RecordClosedCandle resetea el tracker ENTERO (todos los
// simbolos, no solo el que se esta procesando) apenas ve una vela de un dia
// distinto al que ya tenia acumulado, asi que alimentarlo con historial
// viejo (comun en el primer backfill de un simbolo, que puede traer meses)
// lo vaciaria de un tiron para todo el universo. Filtrar aca, antes de
// llamarlo, es lo que hace seguro reusar ese mismo camino para lotes de
// backfill en vez de solo para el stream en vivo.
func TodaysM1Candles(candles []Candle, now time.Time) []Candle {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil
	}
	nowET := now.In(loc)
	today := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)
	tomorrow := today.Add(24 * time.Hour)

	filtered := make([]Candle, 0, len(candles))
	for _, c := range candles {
		if c.Timeframe != M1 {
			continue
		}
		if c.Timestamp.Before(today) || !c.Timestamp.Before(tomorrow) {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}
