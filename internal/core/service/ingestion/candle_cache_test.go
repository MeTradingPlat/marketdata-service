package ingestion

import (
	"strconv"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func newTestCandleCache() *candleCache {
	return &candleCache{entries: make(map[string]candleCacheEntry)}
}

func seriesOf(n int) []domain.Candle {
	return make([]domain.Candle, n)
}

func TestCandleCache_TotalDeVelasNuncaSuperaElTope(t *testing.T) {
	c := newTestCandleCache()
	perEntry := candleCacheMaxCandles / 10

	for i := 0; i < 25; i++ {
		c.put("k"+strconv.Itoa(i), seriesOf(perEntry))
		if c.total > candleCacheMaxCandles {
			t.Fatalf("total=%d supera el tope %d tras %d puts", c.total, candleCacheMaxCandles, i+1)
		}
	}
	if c.total == 0 || len(c.entries) == 0 {
		t.Fatal("el cache no deberia quedar vacio")
	}
}

func TestCandleCache_UnaSerieEnormeNoSeCachea(t *testing.T) {
	c := newTestCandleCache()

	c.put("gigante", seriesOf(candleCacheMaxCandles))

	if _, ok := c.get("gigante"); ok || c.total != 0 {
		t.Fatalf("una serie enorme no debe cachearse (total=%d)", c.total)
	}
}

func TestCandleCache_ReemplazarUnaLlaveNoDuplicaElConteo(t *testing.T) {
	c := newTestCandleCache()

	c.put("AAPL", seriesOf(100))
	c.put("AAPL", seriesOf(40))

	if c.total != 40 || len(c.entries) != 1 {
		t.Fatalf("total=%d entries=%d, want 40 y 1", c.total, len(c.entries))
	}
}

func TestCandleCache_UnaEntradaVencidaDescuentaSuTotalAlLeerse(t *testing.T) {
	c := newTestCandleCache()
	c.put("AAPL", seriesOf(100))
	entry := c.entries["AAPL"]
	entry.expires = entry.expires.Add(-2 * candleCacheTTL)
	c.entries["AAPL"] = entry

	if _, ok := c.get("AAPL"); ok || c.total != 0 {
		t.Fatalf("una entrada vencida debe desaparecer y descontar su total (total=%d)", c.total)
	}
}
