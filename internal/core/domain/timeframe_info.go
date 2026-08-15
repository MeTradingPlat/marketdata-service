package domain

// Mismo formato que TimeframeDTORespuesta del servicio Java (id/codigo/nombre).
type TimeframeInfo struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}
