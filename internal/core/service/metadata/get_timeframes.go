package metadata

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

// Solo M1/H1/D1 -- los unicos timeframes con datos reales guardados hoy.
// Java exponia 21 (incluyendo derivados como M5/H4/semanal), pero listar
// uno que /historical no puede servir todavia rompe el frontend en
// silencio. Se amplia cuando exista la agregacion (ver plan pendiente).
var timeframes = []dto.TimeframeInfo{
	{ID: "M1", Codigo: "1m", Nombre: "1 Minuto"},
	{ID: "H1", Codigo: "1h", Nombre: "1 Hora"},
	{ID: "D1", Codigo: "1d", Nombre: "1 Día"},
}

type getTimeframesService struct{}

func NewGetTimeframesService() in.GetTimeframesService {
	return &getTimeframesService{}
}

func (s *getTimeframesService) GetTimeframes() []dto.TimeframeInfo {
	return timeframes
}
