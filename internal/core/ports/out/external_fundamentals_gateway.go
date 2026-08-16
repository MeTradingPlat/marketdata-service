package out

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// Estos 4 puertos aislan al core de FINRA/SEC EDGAR, igual que
// MarketDataGateway aisla a TastyTrade -- cambiar de fuente el dia de manana
// es un adaptador nuevo en adapters/outgoing/external/, cero cambios aca.

// SharesOutstandingGateway resuelve sharesOutstanding via el bulk
// companyfacts.zip de SEC EDGAR (cacheado en disco, un simbolo sin dato
// simplemente no aparece en el mapa devuelto).
type SharesOutstandingGateway interface {
	FetchSharesOutstanding(ctx context.Context, symbols []string) map[string]int64
}

// InsiderOwnershipGateway resuelve tenencias de insiders (Form 3/4/5) via
// los ZIPs trimestrales bulk de la SEC.
type InsiderOwnershipGateway interface {
	FetchInsiderShares(ctx context.Context, symbols []string) map[string]domain.InsiderOwnership
}

// BeneficialOwnersGateway resuelve holders institucionales de 5%+ (Schedule
// 13D/13G) -- a diferencia de los otros 3 puertos de este archivo, es
// por-simbolo bajo demanda, sin archivo bulk.
type BeneficialOwnersGateway interface {
	FetchBeneficialOwnerShares(ctx context.Context, symbol string, excludeCiks map[string]bool) int64
}

// ShortInterestGateway resuelve shortInterest/shortRatio via el CSV
// biweekly de FINRA.
type ShortInterestGateway interface {
	DownloadLatest(ctx context.Context) map[string]domain.ShortInterestRecord
}
