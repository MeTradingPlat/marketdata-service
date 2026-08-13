package tastytrade

import (
	"math"
	"strconv"
	"time"
)

type rawCandleEvent struct {
	Symbol    string
	Timestamp time.Time
	Open      *float64
	High      *float64
	Low       *float64
	Close     *float64
	Volume    *float64
	VWAP      *float64
}

// parseCandleBatch recorre el arreglo COMPACT de dxLink: cada registro
// arranca con su simbolo (string) de nuevo, y el ancho no es fijo -- campos
// finales con valores default/ausentes pueden faltar por registro.
func parseCandleBatch(data []interface{}) []rawCandleEvent {
	var events []rawCandleEvent
	start := 0
	for start < len(data) {
		end := start + 1
		for end < len(data) {
			if _, isString := data[end].(string); isString {
				break
			}
			end++
		}
		if ev, ok := parseCandleRecord(data[start:end]); ok {
			events = append(events, ev)
		}
		start = end
	}
	return events
}

func parseCandleRecord(record []interface{}) (rawCandleEvent, bool) {
	symbol, ok := record[0].(string)
	if !ok || symbol == "" {
		return rawCandleEvent{}, false
	}
	ms, _ := fieldFloat(record, 1)
	return rawCandleEvent{
		Symbol:    symbol,
		Timestamp: time.UnixMilli(int64(ms)),
		Open:      fieldNullableFloat(record, 2),
		High:      fieldNullableFloat(record, 3),
		Low:       fieldNullableFloat(record, 4),
		Close:     fieldNullableFloat(record, 5),
		Volume:    fieldNullableFloat(record, 6),
		VWAP:      fieldNullableFloat(record, 7),
	}, true
}

func fieldFloat(record []interface{}, idx int) (float64, bool) {
	if idx >= len(record) {
		return 0, false
	}
	switch v := record[idx].(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func fieldNullableFloat(record []interface{}, idx int) *float64 {
	if idx >= len(record) || record[idx] == nil {
		return nil
	}
	f, ok := fieldFloat(record, idx)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}
