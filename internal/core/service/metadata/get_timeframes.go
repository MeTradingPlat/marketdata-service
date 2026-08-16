package metadata

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

// Los 21 timeframes que /historical y /ws/candles ya pueden servir --
// M1/H1/D1 nativos de TastyTrade, el resto agrupado en el momento (ver
// domain.Timeframe.Aggregation).
var timeframes = []dto.TimeframeInfo{
	{ID: "M1", Codigo: "1m", Nombre: "1 Minuto"},
	{ID: "M2", Codigo: "2m", Nombre: "2 Minutos"},
	{ID: "M3", Codigo: "3m", Nombre: "3 Minutos"},
	{ID: "M5", Codigo: "5m", Nombre: "5 Minutos"},
	{ID: "M10", Codigo: "10m", Nombre: "10 Minutos"},
	{ID: "M15", Codigo: "15m", Nombre: "15 Minutos"},
	{ID: "M30", Codigo: "30m", Nombre: "30 Minutos"},
	{ID: "M45", Codigo: "45m", Nombre: "45 Minutos"},
	{ID: "H1", Codigo: "1h", Nombre: "1 Hora"},
	{ID: "H2", Codigo: "2h", Nombre: "2 Horas"},
	{ID: "H3", Codigo: "3h", Nombre: "3 Horas"},
	{ID: "H4", Codigo: "4h", Nombre: "4 Horas"},
	{ID: "H12", Codigo: "12h", Nombre: "12 Horas"},
	{ID: "D1", Codigo: "1d", Nombre: "1 Día"},
	{ID: "D2", Codigo: "2d", Nombre: "2 Días"},
	{ID: "D3", Codigo: "3d", Nombre: "3 Días"},
	{ID: "W1", Codigo: "1w", Nombre: "1 Semana"},
	{ID: "MO1", Codigo: "1mo", Nombre: "1 Mes"},
	{ID: "MO3", Codigo: "3mo", Nombre: "3 Meses"},
	{ID: "MO6", Codigo: "6mo", Nombre: "6 Meses"},
	{ID: "Y1", Codigo: "1y", Nombre: "1 Año"},
}

type getTimeframesService struct{}

func NewGetTimeframesService() in.GetTimeframesService {
	return &getTimeframesService{}
}

func (s *getTimeframesService) GetTimeframes() []dto.TimeframeInfo {
	return timeframes
}
