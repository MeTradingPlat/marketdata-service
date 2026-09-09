package verifybootstrap

import (
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
)

// NewOAuth arma un tastytrade.OAuth desde la config real del servicio --
// evita que cada binario de cmd/verify-* repita el mismo mapeo de 4 campos,
// y evita que alguno quede con una URL o credenciales distintas por error
// (como pasaba antes en verify-d1/verify-depth, con una base URL hardcodeada
// distinta de la real).
func NewOAuth(cfg *configs.Config) *tastytrade.OAuth {
	return tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      cfg.TastyTradeBaseURL,
		ClientID:     cfg.TastyTradeClientID,
		ClientSecret: cfg.TastyTradeClientSecret,
		RefreshToken: cfg.TastyTradeRefreshToken,
	})
}
