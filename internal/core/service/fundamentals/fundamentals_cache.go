package fundamentals

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/rs/zerolog/log"
)

// FundamentalsCache mantiene los fundamentales de todo el universo EN
// MEMORIA -- releer Postgres en cada /marketdata/fundamentals/realtime
// (varios escaneres pidiendo el universo completo cada ~60-90s) seria
// trabajo repetido sobre un dato que ya se tiene. Reactivo de verdad: cada
// refresh puntual (beta/market metrics/earnings/dividendos/externo/
// prevClose/prevPostMarketVolume) llama a su Merge* correspondiente apenas
// Postgres confirma la escritura (ver mas abajo), asi que el cache queda al
// dia EN EL MOMENTO en que cambia el dato, no en la proxima corrida
// periodica. ReloadAll() sigue existiendo solo para el arranque en frio
// (Postgres es el respaldo del que se reconstruye el cache tras un
// reinicio) -- fuera de eso, nada vuelve a pedirle a Postgres lo que el
// cache ya tiene.
type FundamentalsCache struct {
	repo        out.FundamentalsRepository
	symbols     out.SymbolRepository
	broadcaster *livecandles.Broadcaster[domain.Fundamentals]

	mu   sync.RWMutex
	data map[string]domain.Fundamentals
}

func NewFundamentalsCache(repo out.FundamentalsRepository, symbols out.SymbolRepository, broadcaster *livecandles.Broadcaster[domain.Fundamentals]) *FundamentalsCache {
	return &FundamentalsCache{repo: repo, symbols: symbols, broadcaster: broadcaster, data: make(map[string]domain.Fundamentals)}
}

// ReloadAll relee TODO el universo tracked de una sola consulta y reemplaza
// el cache entero -- SOLO para el arranque en frio (cache vacio, recien
// creado el proceso): a partir de ahi cada refresh mantiene el cache al dia
// solo (ver los Merge* mas abajo), asi que esto no necesita volver a
// llamarse durante el resto del dia. Un error deja el cache anterior
// intacto en vez de vaciarlo.
func (c *FundamentalsCache) ReloadAll(ctx context.Context) {
	tracked, err := c.symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("fundamentals cache reload: fetching tracked symbols failed, keeping previous data")
		return
	}
	names := make([]string, len(tracked))
	for i, s := range tracked {
		names[i] = s.Symbol
	}
	data, err := c.repo.GetBatch(ctx, names)
	if err != nil {
		log.Error().Err(err).Msg("fundamentals cache reload failed, keeping previous data")
		return
	}
	c.mu.Lock()
	c.data = data
	c.mu.Unlock()

	c.publishAll(data)
}

// publishAll manda cada fundamental recien recargado a quien este
// suscripto a /ws/fundamentals -- solo se usa desde ReloadAll (arranque en
// frio); el resto del dia cada Merge* ya publica lo suyo individualmente
// via merge().
func (c *FundamentalsCache) publishAll(data map[string]domain.Fundamentals) {
	if c.broadcaster == nil {
		return
	}
	for symbol, f := range data {
		c.broadcaster.Publish(symbol, f)
	}
}

// Cada Merge* de aca abajo es el equivalente EN MEMORIA de su Upsert*
// homonimo en FundamentalsRepository -- write-through real: el cache queda
// al dia en el momento en que Postgres confirma la escritura, sin esperar al
// proximo ReloadAll periodico. Postgres sigue siendo la fuente de verdad
// para sobrevivir un reinicio (ReloadAll sigue siendo el unico camino para
// el arranque en frio); esto solo evita que el cache quede desactualizado
// ENTRE reinicios. Cada uno pisa SOLO los campos que su Upsert* escribe,
// nunca el resto del registro -- son refrescos independientes (beta,
// dividendos, market metrics, earnings, externo, prevClose,
// prevPostMarketVolume) que conviven en el mismo symbol, y un reemplazo
// completo del registro borraria lo que otro refresh ya habia llenado.
func (c *FundamentalsCache) merge(symbol string, patch func(*domain.Fundamentals)) {
	c.mu.Lock()
	f := c.data[symbol]
	f.Symbol = symbol
	patch(&f)
	c.data[symbol] = f
	c.mu.Unlock()

	if c.broadcaster != nil {
		c.broadcaster.Publish(symbol, f)
	}
}

// MergeDividends -- ver UpsertDividends, mismo set de columnas, sin
// COALESCE: la respuesta de /market-data/by-type siempre pisa entero.
func (c *FundamentalsCache) MergeDividends(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			f.DividendAmount = u.DividendAmount
			f.DividendFrequency = u.DividendFrequency
			f.TradingStatus = u.TradingStatus
			f.StatusReason = u.StatusReason
			f.HaltStartTime = u.HaltStartTime
			f.HaltEndTime = u.HaltEndTime
			f.MarketDataUpdatedAt = &now
		})
	}
}

// MergeMarketMetrics -- ver UpsertMarketMetrics, sin COALESCE.
func (c *FundamentalsCache) MergeMarketMetrics(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			f.MarketCap = u.MarketCap
			f.Eps = u.Eps
			f.Beta = u.Beta
			f.Lendability = u.Lendability
			f.BorrowRate = u.BorrowRate
			f.Liquidity = u.Liquidity
			f.LiquidityRating = u.LiquidityRating
			f.ImpliedVolatilityIndex = u.ImpliedVolatilityIndex
			f.ImpliedVolatilityRank = u.ImpliedVolatilityRank
			f.ImpliedVolatilityPercentile = u.ImpliedVolatilityPercentile
			f.NextEarningsDate = u.NextEarningsDate
			f.MetricsUpdatedAt = &now
		})
	}
}

// MergeBeta -- ver UpsertBeta, sin COALESCE.
func (c *FundamentalsCache) MergeBeta(updates []domain.Fundamentals) {
	for _, u := range updates {
		beta := u.Beta
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			f.Beta = beta
		})
	}
}

// MergeEarningsHistory -- ver UpsertEarningsHistory: NULLIF+COALESCE en SQL
// (una fecha vacia no pisa una vigente), mismo criterio aca con "" en vez
// de NULL (ver el comentario de domain.Fundamentals.OccurredDate sobre por
// que "" siempre significa "sin dato").
func (c *FundamentalsCache) MergeEarningsHistory(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			if u.OccurredDate != "" {
				f.OccurredDate = u.OccurredDate
			}
			if u.NextEarningsDate != "" {
				f.NextEarningsDate = u.NextEarningsDate
			}
			f.EarningsUpdatedAt = &now
		})
	}
}

// MergeExternalFundamentals -- ver UpsertExternalFundamentals: COALESCE en
// TODOS los campos puntero de SQL, mismo criterio aca (nil no pisa lo que ya
// habia). ShortInterestSettlement es string, no puntero, igual que en la
// fila de Postgres (nunca es NULL de verdad) -- siempre se pisa.
func (c *FundamentalsCache) MergeExternalFundamentals(updates []domain.Fundamentals) {
	now := time.Now()
	for _, u := range updates {
		c.merge(u.Symbol, func(f *domain.Fundamentals) {
			if u.SharesOutstanding != nil {
				f.SharesOutstanding = u.SharesOutstanding
			}
			if u.FloatShares != nil {
				f.FloatShares = u.FloatShares
			}
			if u.ShortInterest != nil {
				f.ShortInterest = u.ShortInterest
			}
			if u.ShortRatio != nil {
				f.ShortRatio = u.ShortRatio
			}
			if u.ShortInterestShares != nil {
				f.ShortInterestShares = u.ShortInterestShares
			}
			f.ShortInterestSettlement = u.ShortInterestSettlement
			if u.InsiderShares != nil {
				f.InsiderShares = u.InsiderShares
			}
			if u.InsiderCiks != nil {
				f.InsiderCiks = u.InsiderCiks
			}
			if u.FloatUpdatedAt != nil {
				f.FloatUpdatedAt = u.FloatUpdatedAt
			}
			f.ExternalUpdatedAt = &now
		})
	}
}

// MergePrevClose -- ver UpsertPrevCloseBatch: closes trae el valor real,
// attemptedOnly son simbolos sin dato (warrant sin M1, etc.) que igual
// avanzan PrevCloseUpdatedAt para no repetir la busqueda esta ventana (ver
// el comentario de domain.Fundamentals.PrevCloseUpdatedAt).
func (c *FundamentalsCache) MergePrevClose(closes map[string]float64, attemptedOnly []string) {
	now := time.Now()
	for symbol, value := range closes {
		v := value
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevClose = &v
			f.PrevCloseUpdatedAt = &now
		})
	}
	for _, symbol := range attemptedOnly {
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevCloseUpdatedAt = &now
		})
	}
}

// MergePrevPostMarketVolume -- mismo criterio que MergePrevClose, ver
// UpsertPrevPostMarketVolumeBatch.
func (c *FundamentalsCache) MergePrevPostMarketVolume(volumes map[string]int64, attemptedOnly []string) {
	now := time.Now()
	for symbol, value := range volumes {
		v := value
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevPostMarketVolume = &v
			f.PrevPostMarketVolumeUpdatedAt = &now
		})
	}
	for _, symbol := range attemptedOnly {
		c.merge(symbol, func(f *domain.Fundamentals) {
			f.PrevPostMarketVolumeUpdatedAt = &now
		})
	}
}

// GetBatch sirve del cache en memoria -- un simbolo sin fundamentales
// conocidos simplemente no aparece, mismo contrato que FundamentalsRepository.GetBatch.
func (c *FundamentalsCache) GetBatch(symbols []string) map[string]domain.Fundamentals {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]domain.Fundamentals, len(symbols))
	for _, s := range symbols {
		if f, ok := c.data[s]; ok {
			result[s] = f
		}
	}
	return result
}
