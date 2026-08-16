package dto

// Formato distinto al de domain.Candle (que ya tiene sus propios tags json
// para /historical): el frontend espera time en unix-seconds, no timestamp
// ISO, y un campo closed que domain.Candle no tiene -- por eso este es un
// dto propio en vez de reusar los tags de Candle (ver skill
// go-hexagonal-standards sobre cuando algo pertenece a dto en vez de domain).
type CandleBar struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
	Closed bool    `json:"closed"`
}

type CandleHistoryMessage struct {
	Type      string      `json:"type"`
	Symbol    string      `json:"symbol"`
	Timeframe string      `json:"timeframe"`
	Bars      []CandleBar `json:"bars"`
}

type CandleBarMessage struct {
	Type      string    `json:"type"`
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	Bar       CandleBar `json:"bar"`
}

type CandleControlMessage struct {
	Type      string `json:"type"`
	Symbol    string `json:"symbol,omitempty"`
	Timeframe string `json:"timeframe,omitempty"`
	Action    string `json:"action,omitempty"`
	Message   string `json:"message,omitempty"`
}
