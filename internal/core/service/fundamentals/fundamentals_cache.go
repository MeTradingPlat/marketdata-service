package fundamentals

import (
	"context"
	"sync"

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
