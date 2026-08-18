package ingestion

import (
	"context"
	"fmt"
	"sync"
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
	liveMu      sync.RWMutex
	live        map[string]bool
	attempted   map[string]bool
}

func NewIngestCandlesService(gateway out.MarketDataGateway, repo out.CandleRepository, broadcaster *livecandles.Broadcaster) in.IngestCandlesService {
	return &ingestCandlesService{gateway: gateway, repo: repo, broadcaster: broadcaster, retryBuffer: newSaveRetryBuffer(), live: make(map[string]bool), attempted: make(map[string]bool)}
}

// IncrementalMargin son barras de mas antes del watermark que se vuelven a
// pedir aposta -- cubre cualquier borde raro (ej. el watermark se capturo a
// mitad de un guardado). Reducido de 10 a 1 para que ningun re-fetch
// atrasado intente upsertear un chunk ya comprimido (la compresion baja a 2
// dias, y las velas verificadas del refill ya no se re-escriben -- pedir de
// mas era gratis cuando no habia compresion agresiva, ya no). Exportado
// para que el barrido en lote (catchup) use el mismo margen sin duplicarlo.
const IncrementalMargin = 1

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
	if err := s.repo.Save(ctx, closed, true); err != nil {
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
	if err := s.gateway.SubscribeLiveCandles(ctx, symbol, from, func(c domain.Candle) {
		if err := s.repo.Save(ctx, []domain.Candle{c}, false); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to save live candle, buffering for retry")
			s.retryBuffer.add(c)
		}
		s.broadcaster.Publish(c)
	}, func(c domain.Candle) {
		// La vela M1 en formacion tras cada tick -- el WS de /ws/candles la
		// usa para dibujar la vela actual (M1 directo, H1/D1 por
		// agregacion). Sin publicador en el Broadcaster no habria forma de
		// que una sesion WS vea la vela a medio formar: solo se publicaba
		// al cerrar.
		s.broadcaster.Publish(c)
	}); err != nil {
		s.setLive(symbol, false)
		return err
	}
	s.setLive(symbol, true)
	return nil
}

func (s *ingestCandlesService) setLive(symbol string, live bool) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.live[symbol] = live
	s.attempted[symbol] = true
}

// IsLive dice si el stream en vivo del simbolo arranco.
func (s *ingestCandlesService) IsLive(symbol string) bool {
	s.liveMu.RLock()
	defer s.liveMu.RUnlock()
	return s.live[symbol]
}

// IsAttempted distingue "el rollout lo intento y fallo" de "nadie lo ha
// intentado todavia" -- el reconciliador (cmd/api) solo reintenta los
// intentados-fallidos; reintentar los nunca-intentados en el primer tick
// (5 min tras boot, cuando el ciclo recien va por D1/beta) re-registra los
// dispatch entries del rollout en curso y lo frena (confirmado en vivo el
// 2026-08-18 18:07-18:20: el ciclo quedo atascado 15+ min sin abrir).
func (s *ingestCandlesService) IsAttempted(symbol string) bool {
	s.liveMu.RLock()
	defer s.liveMu.RUnlock()
	return s.attempted[symbol]
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
	if err := s.repo.Save(ctx, pending, false); err != nil {
		log.Error().Err(err).Int("candles", len(pending)).Msg("retrying pending candle saves failed, will retry again")
		s.retryBuffer.requeue(pending)
		return
	}
	log.Info().Int("candles", len(pending)).Msg("recovered pending candle saves")
}
