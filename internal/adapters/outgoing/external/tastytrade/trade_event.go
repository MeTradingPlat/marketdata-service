package tastytrade

// tradeEventFields: solo lo que FetchDayVolumes necesita -- dayVolume es
// el volumen consolidado real del dia (confirmado en vivo el 2026-09-22
// contra /market-data/by-type), a diferencia de Candle.volume que sale de
// una fuente mas angosta y solo trae 40-60% de eso.
var tradeEventFields = []string{"eventSymbol", "dayVolume"}

type rawTradeEvent struct {
	Symbol    string
	DayVolume *float64
}

// parseTradeBatch recorre el arreglo COMPACT de dxLink -- mismo formato
// que velas/perfil (ver parseCandleBatch/parseProfileBatch), cada registro
// arranca de nuevo con su simbolo.
func parseTradeBatch(data []interface{}) []rawTradeEvent {
	var events []rawTradeEvent
	start := 0
	for start < len(data) {
		end := start + 1
		for end < len(data) {
			if _, isString := data[end].(string); isString {
				break
			}
			end++
		}
		if ev, ok := parseTradeRecord(data[start:end]); ok {
			events = append(events, ev)
		}
		start = end
	}
	return events
}

// El orden de campos en cada registro sigue exactamente el orden declarado
// en tradeEventFields -- indice 0 es el simbolo, 1 es "dayVolume".
func parseTradeRecord(record []interface{}) (rawTradeEvent, bool) {
	symbol, ok := record[0].(string)
	if !ok || symbol == "" {
		return rawTradeEvent{}, false
	}
	return rawTradeEvent{Symbol: symbol, DayVolume: fieldNullableFloat(record, 1)}, true
}
