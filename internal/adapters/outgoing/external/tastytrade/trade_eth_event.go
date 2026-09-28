package tastytrade

// tradeETHEventFields: dayVolumeAsDouble es el volumen acumulado SOLO de
// horario extendido (pre/post-market) que dxFeed mantiene aparte del
// dayVolume de Trade (que es el volumen oficial de sesion regular) --
// confirmado contra la documentacion de dxFeed (TradeETH.dayVolumeAsDouble).
var tradeETHEventFields = []string{"eventSymbol", "dayVolumeAsDouble"}

type rawTradeETHEvent struct {
	Symbol            string
	ExtendedDayVolume *float64
}

// parseTradeETHBatch/parseTradeETHRecord: mismo formato COMPACT que
// parseTradeBatch/parseTradeRecord (ver trade_event.go), cada registro
// arranca de nuevo con su simbolo.
func parseTradeETHBatch(data []interface{}) []rawTradeETHEvent {
	var events []rawTradeETHEvent
	start := 0
	for start < len(data) {
		end := start + 1
		for end < len(data) {
			if _, isString := data[end].(string); isString {
				break
			}
			end++
		}
		if ev, ok := parseTradeETHRecord(data[start:end]); ok {
			events = append(events, ev)
		}
		start = end
	}
	return events
}

// El orden de campos sigue tradeETHEventFields -- indice 0 es el simbolo,
// 1 es "dayVolumeAsDouble".
func parseTradeETHRecord(record []interface{}) (rawTradeETHEvent, bool) {
	symbol, ok := record[0].(string)
	if !ok || symbol == "" {
		return rawTradeETHEvent{}, false
	}
	return rawTradeETHEvent{Symbol: symbol, ExtendedDayVolume: fieldNullableFloat(record, 1)}, true
}
