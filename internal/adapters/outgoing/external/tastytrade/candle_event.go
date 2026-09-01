package tastytrade

import (
	"math"
	"strconv"
	"time"
)

// Bits de eventFlags para un IndexedEvent de dxFeed (Candle lo es) -- ver
// kb.dxfeed.com/en/data-model/market-events/qd-model-of-market-events.html.
// snapshotEnd: el servidor mando TODO lo que hay. snapshotSnip: el servidor
// corto la rafaga por su propio limite (a diferencia de nuestro timeout,
// esto es una señal real y honesta de "no hay mas"). txPending: todavia
// hay mas eventos de esta MISMA actualizacion transaccional en camino, no
// procesar como definitivo aunque venga con snapshotEnd/snapshotSnip.
const (
	eventFlagTxPending    = 1 << 0
	eventFlagSnapshotEnd  = 1 << 3
	eventFlagSnapshotSnip = 1 << 4
)

type rawCandleEvent struct {
	Symbol     string
	Timestamp  time.Time
	Open       *float64
	High       *float64
	Low        *float64
	Close      *float64
	Volume     *float64
	VWAP       *float64
	EventFlags int
}

// snapshotDone es verdadero solo cuando el servidor marco el final real de
// la rafaga (SNAPSHOT_END o SNAPSHOT_SNIP) Y no queda ninguna actualizacion
// transaccional pendiente -- ver historyCollector.settled(), que usa esto
// en vez de (ademas de) el timeout de reloj cuando el campo llega poblado.
func (e rawCandleEvent) snapshotDone() bool {
	return e.EventFlags&eventFlagTxPending == 0 &&
		e.EventFlags&(eventFlagSnapshotEnd|eventFlagSnapshotSnip) != 0
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
	flags, _ := fieldFloat(record, 9)
	return rawCandleEvent{
		Symbol:     symbol,
		Timestamp:  time.UnixMilli(int64(ms)),
		Open:       fieldNullableFloat(record, 2),
		High:       fieldNullableFloat(record, 3),
		Low:        fieldNullableFloat(record, 4),
		Close:      fieldNullableFloat(record, 5),
		Volume:     fieldNullableFloat(record, 6),
		VWAP:       fieldNullableFloat(record, 7),
		EventFlags: int(flags),
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
