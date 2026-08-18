package out

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type CandleRepository interface {
	// Save persiste velas; withWatermark controla si se avanza el watermark
	// del simbolo+timeframe. Las velas EN VIVO (cerradas minuto a minuto
	// desde la vela en formacion) guardan con false -- su watermark solo lo
	// avanza el refill/backfill (las velas reales pedidas a TastyTrade);
	// asi un reinicio vuelve a pedir exactamente lo que falta desde el
	// ultimo watermark del refill.
	Save(ctx context.Context, candles []domain.Candle, withWatermark bool) error
	// before es nil para las barras mas recientes, o un limite exclusivo
	// para paginar hacia atras (ver CandleRepository.GetCandles).
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error)
	// GetSeries lee hasta bars velas por simbolo para un lote de simbolos en
	// una sola query -- los refrescos masivos (beta) no deben correr una
	// query por simbolo: satura postgres y las escrituras M1 en vivo hacen
	// fila detras (ver RefreshBeta).
	GetSeries(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) (map[string][]domain.Candle, error)
	GetWatermark(ctx context.Context, symbol string, timeframe domain.Timeframe) (newest *time.Time, err error)
	// GetWatermarksBatch lee los watermarks de un lote de simbolos en UNA
	// query (el barrido leia uno por uno: 13k queries por fase, la mayor
	// parte del tiempo del refill -- ver backfillBatchWithRetry).
	GetWatermarksBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe) (map[string]time.Time, error)
	SymbolsWithData(ctx context.Context, timeframe domain.Timeframe) (map[string]struct{}, error)
	// GetIntradaySessions agrega las velas M1 de hoy por sesion (pre-market,
	// regular, post-market) -- devuelve solo esos campos de
	// IntradaySnapshot, el resto (precio/volumen actual, prevClose) los
	// completa el caso de uso desde otras fuentes.
	GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error)
	// GetPreviousSessionClose devuelve el cierre REGULAR (subasta 16:00 ET)
	// de la sesion anterior a before -- busca la ultima vela M1 cuya hora ET
	// cae en [15:58, 16:02) de alguno de los ultimos 10 dias de calendario,
	// asi que salta fines de semana y feriados por construccion (no hay
	// aritmetica de "dia anterior"). (nil, nil) si no existe ninguno.
	GetPreviousSessionClose(ctx context.Context, symbol string, before time.Time) (*float64, error)
}

type SymbolRepository interface {
	Upsert(ctx context.Context, symbols []domain.Symbol) error
	Tracked(ctx context.Context) ([]domain.Symbol, error)
	GetBySymbol(ctx context.Context, symbol string) (domain.Symbol, error)
	// GetBatch es una sola consulta acotada por symbol = ANY(...), no N
	// consultas individuales -- para /marketdata/fundamentals/realtime.
	GetBatch(ctx context.Context, symbols []string) (map[string]domain.Symbol, error)
	// Search ordena por last_volume descendente (ver Save() en el
	// repositorio) en vez de alfabetico -- mas relevante para un screener.
	Search(ctx context.Context, query string, markets []string, page, size int) (symbols []domain.Symbol, totalElements int64, err error)
	Deactivate(ctx context.Context, symbols []string) error
	Markets(ctx context.Context) ([]string, error)
}

type FundamentalsRepository interface {
	Get(ctx context.Context, symbol string) (domain.Fundamentals, error)
	// GetBatch es una sola consulta acotada por symbol = ANY(...), no N
	// consultas individuales -- para /marketdata/fundamentals/realtime.
	GetBatch(ctx context.Context, symbols []string) (map[string]domain.Fundamentals, error)
	UpsertDividends(ctx context.Context, fundamentals []domain.Fundamentals) error
	UpsertMarketMetrics(ctx context.Context, fundamentals []domain.Fundamentals) error
	// UpsertExternalFundamentals cubre sharesOutstanding/floatShares (SEC
	// EDGAR) y shortInterest/shortRatio (FINRA) -- fuentes ajenas a
	// TastyTrade, refrescadas juntas. Cada columna usa COALESCE contra el
	// valor ya guardado, asi que un caller que solo llena un subconjunto de
	// campos (ej. RefreshBeneficialOwners solo toca floatShares) no borra lo
	// que puso otro caller.
	UpsertExternalFundamentals(ctx context.Context, fundamentals []domain.Fundamentals) error
	// GetSymbolsDueForFloatRefresh trae el lote de simbolos con
	// sharesOutstanding+insiderShares ya conocidos pero floatShares real
	// menos reciente -- ver RefreshBeneficialOwners.
	GetSymbolsDueForFloatRefresh(ctx context.Context, limit int) ([]domain.Fundamentals, error)
	// UpsertEarningsHistory cubre occurredDate (ultimo reporte real) y,
	// cuando hay una prediccion valida, nextEarningsDate -- ver
	// RefreshEarningsHistory.
	UpsertEarningsHistory(ctx context.Context, fundamentals []domain.Fundamentals) error
	// GetSymbolsWithStaleEarnings trae los simbolos sin nextEarningsDate o
	// con uno ya vencido -- el endpoint de earnings history es por-simbolo,
	// esto acota el refresco a los que de verdad lo necesitan.
	GetSymbolsWithStaleEarnings(ctx context.Context) ([]string, error)
	// UpsertBeta pisa beta con el calculado desde velas propias (5Y monthly
	// contra SPY, ver domain.MonthlyBeta) SOLO para los simbolos incluidos
	// -- los que no pudieron calcularse conservan el beta de TastyTrade.
	UpsertBeta(ctx context.Context, fundamentals []domain.Fundamentals) error
	// RecordStepDone registra cuando termino un refresh diario de
	// fundamentales -- el ciclo salta el paso en reinicios dentro de la
	// misma ventana de mantenimiento (ver refreshFundamentalsOnce).
	RecordStepDone(ctx context.Context, step string, at time.Time) error
	// StepDoneAt devuelve cuando se calculo por ultima vez el paso (false
	// si nunca se calculo).
	StepDoneAt(ctx context.Context, step string) (time.Time, bool, error)
	// GetSymbolsWithStaleBeta trae los simbolos cuyo beta no se calculo en
	// la ventana de mantenimiento actual (guard por-simbolo con fecha).
	GetSymbolsWithStaleBeta(ctx context.Context, windowStart time.Time) ([]string, error)
	// GetSymbolsWithStalePrevClose trae los simbolos cuyo prevClose no se
	// calculo en la ventana actual (mismo guard por-simbolo).
	GetSymbolsWithStalePrevClose(ctx context.Context, windowStart time.Time) ([]string, error)
	// UpsertPrevClose guarda el prevClose del backfill con su fecha.
	UpsertPrevClose(ctx context.Context, symbol string, close float64) error
	// MarkPrevCloseAttempted estampa prev_close_updated_at sin tocar
	// prev_close -- "se intento y no hay dato" (simbolo sin M1) no debe
	// re-procesarse en cada ciclo (3,587 simbolos x ~10 min por ciclo,
	// confirmado en vivo el 2026-08-18).
	MarkPrevCloseAttempted(ctx context.Context, symbol string) error
}
