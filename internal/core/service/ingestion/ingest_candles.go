package ingestion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

type ingestCandlesService struct {
	gateway             out.MarketDataGateway
	repo                out.CandleRepository
	broadcaster         *livecandles.Broadcaster[domain.Candle]
	snapshotBroadcaster *livecandles.Broadcaster[domain.IntradaySnapshot]
	tracker             *intraday.SnapshotTracker
	recentCache         *livecandles.RecentCache
	spool               out.CandleSpool
	liveSaveBuffer      *liveSaveBuffer
	retryBuffer         *saveRetryBuffer
	liveMu              sync.RWMutex
	live                map[string]bool
	attempted           map[string]bool
}

func NewIngestCandlesService(gateway out.MarketDataGateway, repo out.CandleRepository, broadcaster *livecandles.Broadcaster[domain.Candle], snapshotBroadcaster *livecandles.Broadcaster[domain.IntradaySnapshot], tracker *intraday.SnapshotTracker, recentCache *livecandles.RecentCache, spool out.CandleSpool) in.IngestCandlesService {
	return &ingestCandlesService{gateway: gateway, repo: repo, broadcaster: broadcaster, snapshotBroadcaster: snapshotBroadcaster, tracker: tracker, recentCache: recentCache, spool: spool, liveSaveBuffer: newLiveSaveBuffer(), retryBuffer: newSaveRetryBuffer(spool), live: make(map[string]bool), attempted: make(map[string]bool)}
}

var ErrEmptyBackfill = errors.New("backfill returned no candles")

// IncrementalMargin son barras de mas antes del watermark que se vuelven a
// pedir aposta -- cubre cualquier borde raro (ej. el watermark se capturo a
// mitad de un guardado). Reducido de 10 a 1 para que ningun re-fetch
// atrasado intente upsertear un chunk ya comprimido (la compresion baja a 2
// dias, y las velas verificadas del refill ya no se re-escriben -- pedir de
// mas era gratis cuando no habia compresion agresiva, ya no). Exportado
// para que el barrido en lote (catchup) use el mismo margen sin duplicarlo.
const IncrementalMargin = 1

func (s *ingestCandlesService) Backfill(ctx context.Context, symbol string, timeframe domain.Timeframe) error {
	candles, requested, err := s.fetchForBackfill(ctx, symbol, timeframe)
	if err != nil {
		if errors.Is(err, domain.ErrPartialHistory) {
			closed, cerr := closedCandles(candles, timeframe)
			if cerr != nil {
				return cerr
			}
			if len(closed) > 0 {
				if serr := s.repo.Save(ctx, closed, false); serr != nil {
					return fmt.Errorf("saving backfilled candles for %s %s: %w", symbol, timeframe, serr)
				}
				s.RecordTodaysClosedCandles(closed)
			}
			return fmt.Errorf("backfill for %s %s: %w", symbol, timeframe, err)
		}
		return err
	}
	if requested && len(candles) == 0 {
		return fmt.Errorf("%s %s: %w", symbol, timeframe, ErrEmptyBackfill)
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
	s.RecordTodaysClosedCandles(closed)
	return nil
}

// RecordTodaysClosedCandles ver el comentario del puerto (in.IngestCandlesService)
// sobre por que existe. TodaysM1Candles hace el filtro que lo vuelve seguro
// reusar RecordClosedCandle (que resetea el tracker entero ante una vela de
// otro dia) para lotes de backfill que pueden traer meses de historia.
func (s *ingestCandlesService) RecordTodaysClosedCandles(candles []domain.Candle) {
	for _, c := range domain.TodaysM1Candles(candles, time.Now()) {
		s.tracker.RecordClosedCandle(c)
	}
}

// fetchForBackfill pide profundidad completa solo la primera vez (sin
// watermark todavia); de ahi en adelante solo pide desde el ultimo dato
// guardado, no lo repite todo. Este mismo camino es el que usa el catch-up
// diario, ya no hace falta agregar M1 -> H1/D1 por separado.
func (s *ingestCandlesService) fetchForBackfill(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, bool, error) {
	newest, err := s.repo.GetWatermark(ctx, symbol, timeframe)
	if err != nil {
		return nil, false, fmt.Errorf("checking watermark for %s %s: %w", symbol, timeframe, err)
	}
	if newest == nil {
		candles, err := s.gateway.ProbeMaxDepth(ctx, symbol, timeframe)
		if err != nil {
			return nil, false, fmt.Errorf("probing max depth for %s %s: %w", symbol, timeframe, err)
		}
		return candles, true, nil
	}
	if timeframe == domain.M1 && s.gateway.LiveSubscribed(symbol) {
		return nil, false, nil
	}
	duration, err := timeframe.Duration()
	if err != nil {
		return nil, false, fmt.Errorf("getting duration for %s: %w", timeframe, err)
	}
	if !HasNewClosedBar(*newest, duration) {
		return nil, false, nil
	}
	from := newest.Add(-IncrementalMargin * duration)
	candles, err := s.gateway.GetCandles(ctx, symbol, timeframe, from)
	if err != nil {
		if errors.Is(err, domain.ErrPartialHistory) {
			return candles, true, fmt.Errorf("fetching incremental candles for %s %s: %w", symbol, timeframe, err)
		}
		return nil, false, fmt.Errorf("fetching incremental candles for %s %s: %w", symbol, timeframe, err)
	}
	return candles, true, nil
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
