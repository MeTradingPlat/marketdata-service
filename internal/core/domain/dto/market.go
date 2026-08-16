package dto

// Market solo tiene forma de respuesta HTTP (catalogo de mercados
// rastreados) -- no participa en ninguna logica de negocio interna, por eso
// vive en dto en vez de domain (ver skill go-hexagonal-standards).
// Mismo formato que MercadoDTORespuesta del servicio Java (id/nombre).
type Market struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}
