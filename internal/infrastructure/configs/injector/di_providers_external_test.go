package injector

import (
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
)

func TestProvideCandlePool(t *testing.T) {
	cfg := &configs.Config{
		MaxCandlePoolConnections:   2,
		MaxChannelsPerConn:         4,
		HistorySubChangesPerMinute: 5000,
		DxlinkURLOverride:          "wss://example.invalid",
	}
	oauth := provideOAuth(cfg)
	qt := tastytrade.NewQuoteToken(oauth)
	pool := provideCandlePool(cfg, qt, oauth)
	if pool == nil {
		t.Fatal("expected non-nil CandlePool")
	}
}
