package tastytrade

type rawProfileEvent struct {
	Symbol string
	Shares *float64
}

// parseProfileBatch recorre el arreglo COMPACT de dxLink -- mismo formato
// que las velas (ver parseCandleBatch), cada registro arranca de nuevo con
// su simbolo.
func parseProfileBatch(data []interface{}) []rawProfileEvent {
	var events []rawProfileEvent
	start := 0
	for start < len(data) {
		end := start + 1
		for end < len(data) {
			if _, isString := data[end].(string); isString {
				break
			}
			end++
		}
		if ev, ok := parseProfileRecord(data[start:end]); ok {
			events = append(events, ev)
		}
		start = end
	}
	return events
}

// El orden de campos en cada registro sigue exactamente el orden declarado
// en profileEventFields (ver dxlink_channel.go) -- indice 0 es el simbolo,
// 1 es "shares".
func parseProfileRecord(record []interface{}) (rawProfileEvent, bool) {
	symbol, ok := record[0].(string)
	if !ok || symbol == "" {
		return rawProfileEvent{}, false
	}
	return rawProfileEvent{Symbol: symbol, Shares: fieldNullableFloat(record, 1)}, true
}
