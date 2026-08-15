package domain

// Los tags json siguen el formato exacto que ya esperaba el frontend del
// servicio Java (ActiveEquityDTORespuesta: symbol/description/listedMarket)
// -- mismo campo en la misma forma, sin capa de traduccion aparte.
type Symbol struct {
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
	Market      string `json:"listedMarket"`
	IsEtf       bool   `json:"isEtf"`
}
