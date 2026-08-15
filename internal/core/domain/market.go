package domain

// Mismo formato que MercadoDTORespuesta del servicio Java (id/nombre).
type Market struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}
