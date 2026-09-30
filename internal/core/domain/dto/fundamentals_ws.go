package dto

// FundamentalsMessage lleva el mismo FundamentalRealtime que ya sirve
// /marketdata/fundamentals/realtime (sharesOutstanding/floatShares/
// shortInterest/shortRatio/daysUntilEarnings incluidos) -- domain.Fundamentals
// crudo se probo primero, pero varios de esos campos son internos
// (json:"-", nunca viajaban) y dejaban a un cliente de /ws/fundamentals sin
// los datos que de verdad necesita. Se manda cuando cualquier refresco puntual
// de FundamentalsCache corre para ese simbolo (beta/market metrics/earnings/
// dividendos/externo/prevClose), no solo en ReloadAll -- sigue siendo baja
// frecuencia porque estos campos no cambian mas seguido que eso (ver
// project_marketdata_fundamentals_known_limits).
type FundamentalsMessage struct {
	Type         string              `json:"type"`
	Symbol       string              `json:"symbol"`
	Fundamentals FundamentalRealtime `json:"fundamentals"`
}

type FundamentalsControlMessage struct {
	Type    string `json:"type"`
	Symbol  string `json:"symbol,omitempty"`
	Message string `json:"message,omitempty"`
}
