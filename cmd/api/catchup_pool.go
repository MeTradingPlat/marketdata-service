package main

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
)

// buildCatchupGateway arma un CandlePool aparte para el barrido D1/H1 del
// catch-up diario -- nunca comparte conexion/canal con las suscripciones M1
// en vivo, asi un barrido pesado sobre el universo completo no puede
// afectar al streaming en tiempo real. El pool crece conexiones solo segun
// las necesita (channelAllocator.allocate, bajo demanda), nunca las abre
// todas de una -- el techo es CatchupPoolConnections, un limite aparte del
// pool en vivo para que ambos combinados no puedan superar lo que el
// sistema (VAIO + la cuenta de TastyTrade) soporta de verdad.
func buildCatchupGateway(cfg *configs.Config, oauth *tastytrade.OAuth, quoteToken *tastytrade.QuoteToken) out.MarketDataGateway {
	urlFunc := quoteToken.DxlinkURL
	if cfg.DxlinkURLOverride != "" {
		urlFunc = func() string { return cfg.DxlinkURLOverride }
	}
	connFactory := func(ctx context.Context) (*tastytrade.DxLinkConn, error) {
		conn := tastytrade.NewDxLinkConn(urlFunc, quoteToken.Token)
		if err := conn.Connect(ctx); err != nil {
			return nil, err
		}
		return conn, nil
	}
	pool := tastytrade.NewCandlePool(connFactory, cfg.CatchupPoolConnections)
	return tastytrade.NewGateway(oauth, pool)
}
