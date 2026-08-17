package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/rs/zerolog/log"
)

type ingestCandlesService struct {
	gateway     out.MarketDataGateway
	repo        out.CandleRepository
	broadcaster *livecandles.Broadcaster
	retryBuffer *saveRetryBuffer
}

func NewIngestCandlesService(gateway out.MarketDataGateway, repo out.CandleRepository, broadcaster *livecandles.Broadcaster) in.IngestCandlesService {
	return &ingestCandlesService{gateway: gateway, repo: repo, broadcaster: broadcaster, retryBuffer: newSaveRetryBuffer()}
}

// IncrementalMargin son barras de mas antes del watermark que se vuelven a
// pedir aposta -- de sobra (Save() las UPSERTea, pedir de mas es gratis) y
// cubre cualquier borde raro (ej. el watermark se capturo a mitad de un
// guardado). Sugerido por el usuario al ver que sin esto, cada backfill
// repetia toda la profundidad historica sin necesidad. Exportado para que
// el barrido en lote (catchup) use el mismo margen sin duplicarlo.
const IncrementalMargin = 10

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
	newest, err := s.repo.GetWatermark(ctx, symbol, timeframe)
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
	if !HasNewClosedBar(*newest, duration) {
		return nil, nil
	}
	from := newest.Add(-IncrementalMargin * duration)
	candles, err := s.gateway.GetCandles(ctx, symbol, timeframe, from)
	if err != nil {
		return nil, fmt.Errorf("fetching incremental candles for %s %s: %w", symbol, timeframe, err)
	}
	return candles, nil
}

// HasNewClosedBar dice si ya cerro al menos un periodo nuevo despues del
// watermark -- si el siguiente periodo todavia no termina (comun en D1/H1,
// donde la mayoria de los reinicios caen a mitad del dia/hora actual), no
// hay nada que pedir todavia. Evita ocupar un canal/conexion real de DxLink
// para una peticion que de antemano sabemos que no va a traer nada nuevo.
// Exportado para el barrido en lote (catchup), que arma los lotes con el
// mismo criterio sin pasar por Backfill uno por uno.
func HasNewClosedBar(watermark time.Time, duration time.Duration) bool {
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
	if _, err := tf.Duration(); err != nil {
		return nil, fmt.Errorf("getting duration for %s: %w", tf, err)
	}
	return domain.ClosedCandles(candles, time.Now()), nil
}

// StreamLive arranca la unica suscripcion M1 del simbolo desde el ultimo
// watermark guardado -- retoma exactamente donde quedo el ultimo dato (sin
// watermark todavia, el gateway pide profundidad maxima) en vez de un
// Backfill(M1) puntual seguido de un StreamLive en vivo aparte: esa
// secuencia (desuscribir y resuscribir la misma clave casi al mismo tiempo)
// dejaba el streaming mudo para siempre sin ningun error. SubscribeLiveCandles
// solo invoca el callback con velas ya cerradas -- el merge de ticks
// parciales es responsabilidad del adaptador, no de este use case.
func (s *ingestCandlesService) StreamLive(ctx context.Context, symbol string) error {
	newest, err := s.repo.GetWatermark(ctx, symbol, domain.M1)
	if err != nil {
		return fmt.Errorf("checking watermark for %s M1: %w", symbol, err)
	}
	var from time.Time
	if newest != nil {
		from = *newest
	}
	return s.gateway.SubscribeLiveCandles(ctx, symbol, from, func(c domain.Candle) {
		if err := s.repo.Save(ctx, []domain.Candle{c}); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to save live candle, buffering for retry")
			s.retryBuffer.add(c)
		}
		s.broadcaster.Publish(c)
	})
}

// RetryPendingSaves reintenta las velas en vivo que fallaron al guardarse --
// llamado periodicamente desde cmd/api, no en el hilo de lectura del
// WebSocket (ver comentario de saveRetryBuffer sobre por que la suscripcion
// DxLink no necesita tocarse para esto).
func (s *ingestCandlesService) RetryPendingSaves(ctx context.Context) {
	pending := s.retryBuffer.drain()
	if len(pending) == 0 {
		return
	}
	if err := s.repo.Save(ctx, pending); err != nil {
		log.Error().Err(err).Int("candles", len(pending)).Msg("retrying pending candle saves failed, will retry again")
		s.retryBuffer.requeue(pending)
		return
	}
	log.Info().Int("candles", len(pending)).Msg("recovered pending candle saves")
}
