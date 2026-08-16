package dto

// TimeframeInfo solo tiene forma de respuesta HTTP (catalogo de timeframes
// soportados) -- distinto de domain.Timeframe, que si es un tipo de negocio
// real con comportamiento (Duration/Valid) usado en toda la ingesta.
// Mismo formato que TimeframeDTORespuesta del servicio Java (id/codigo/nombre).
type TimeframeInfo struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}
