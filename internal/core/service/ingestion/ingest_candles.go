package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

type ingestCandlesService struct {
	gateway out.MarketDataGateway
	repo    out.CandleRepository
}

func NewIngestCandlesService(gateway out.MarketDataGateway, repo out.CandleRepository) in.IngestCandlesService {
	return &ingestCandlesService{gateway: gateway, repo: repo}
}

// incrementalMargin son barras de mas antes del watermark que se vuelven a
// pedir aposta -- de sobra (Save() las UPSERTea, pedir de mas es gratis) y
// cubre cualquier borde raro (ej. el watermark se capturo a mitad de un
// guardado). Sugerido por el usuario al ver que sin esto, cada backfill
// repetia toda la profundidad historica sin necesidad.
const incrementalMargin = 10

func (s *ingestCandlesService) Backfill(ctx context.Context, symbol string, timeframe domain.Timeframe) error {
	candles, err := s.fetchForBackfill(ctx, symbol, timeframe)
	if err != nil {
		return err
	}
	closed, err := closedCandles(candles, timeframe)
	if err != nil {
		return err
	}
	if len(closed) == 0 {
		return nil
	}
	if err := s.repo.Save(ctx, closed); err != nil {
		return fmt.Errorf("saving backfilled candles for %s %s: %w", symbol, timeframe, err)
	}
	return nil
}

// fetchForBackfill pide profundidad completa solo la primera vez (sin
// watermark todavia); de ahi en adelante solo pide desde el ultimo dato
// guardado, no lo repite todo. Este mismo camino es el que usa el catch-up
// diario, ya no hace falta agregar M1 -> H1/D1 por separado.
func (s *ingestCandlesService) fetchForBackfill(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, error) {
	newest, _, err := s.repo.GetWatermark(ctx, symbol, timeframe)
	if err != nil {
		return nil, fmt.Errorf("checking watermark for %s %s: %w", symbol, timeframe, err)
	}
	if newest == nil {
		candles, err := s.gateway.ProbeMaxDepth(ctx, symbol, timeframe)
		if err != nil {
			return nil, fmt.Errorf("probing max depth for %s %s: %w", symbol, timeframe, err)
		}
		return candles, nil
	}
	duration, err := timeframe.Duration()
	if err != nil {
		return nil, fmt.Errorf("getting duration for %s: %w", timeframe, err)
	}
	if !hasNewClosedBar(*newest, duration) {
		return nil, nil
	}
	from := newest.Add(-incrementalMargin * duration)
	candles, err := s.gateway.GetCandles(ctx, symbol, timeframe, from)
	if err != nil {
		return nil, fmt.Errorf("fetching incremental candles for %s %s: %w", symbol, timeframe, err)
	}
	return candles, nil
}

// hasNewClosedBar dice si ya cerro al menos un periodo nuevo despues del
// watermark -- si el siguiente periodo todavia no termina (comun en D1/H1,
// donde la mayoria de los reinicios caen a mitad del dia/hora actual), no
// hay nada que pedir todavia. Evita ocupar un canal/conexion real de DxLink
// para una peticion que de antemano sabemos que no va a traer nada nuevo.
func hasNewClosedBar(watermark time.Time, duration time.Duration) bool {
	nextBar := watermark.Add(duration)
	return !nextBar.Add(duration).After(time.Now())
}

// closedCandles descarta la vela mas reciente si su periodo todavia no
// termino -- IsComplete() solo confirma que el OHLC no es null, pero
// dxLink manda OHLC parcial de la vela EN FORMACION igual que de las ya
// cerradas (confirmado en vivo: quedo guardada la barra D1 de hoy a mitad
// del dia de trading). El backfill es historico por definicion, nunca
// debe incluir la barra que todavia se esta formando.
func closedCandles(candles []domain.Candle, tf domain.Timeframe) ([]domain.Candle, error) {
	duration, err := tf.Duration()
	if err != nil {
		return nil, fmt.Errorf("getting duration for %s: %w", tf, err)
	}
	now := time.Now()
	closed := make([]domain.Candle, 0, len(candles))
	for _, c := range candles {
		if !c.Timestamp.Add(duration).After(now) {
			closed = append(closed, c)
		}
	}
	return closed, nil
}

// SubscribeLiveCandles solo invoca el callback con velas ya cerradas -- ver
// MarketDataGateway, el merge de ticks parciales es responsabilidad del
// adaptador, no de este use case.
func (s *ingestCandlesService) StreamLive(ctx context.Context, symbol string) error {
	return s.gateway.SubscribeLiveCandles(ctx, symbol, func(c domain.Candle) {
		if err := s.repo.Save(ctx, []domain.Candle{c}); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to save live candle")
		}
	})
}
