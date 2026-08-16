package secedgar

import (
	"context"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const insiderZipURLTemplate = "https://www.sec.gov/files/structureddata/data/insider-transactions-data-sets/%dq%d_form345.zip"

// insiderLookbackQuarters: un insider que no opero en el trimestre mas
// reciente simplemente no aparece en ese ZIP -- su tenencia real sigue
// siendo la de su ultimo Form 4, que puede ser de trimestres anteriores.
// Confirmado con datos reales de Java: de 14 directores de una empresa de
// prueba, solo 2 habian operado en el trimestre mas reciente.
const insiderLookbackQuarters = 8

// InsiderOwnershipClient lee insiders (Form 3/4/5) de un ZIP trimestral
// bulk de la SEC -- cero requests por simbolo, igual que companyfacts.zip.
type InsiderOwnershipClient struct {
	cacheDir string
}

func NewInsiderOwnershipClient(cacheDir string) *InsiderOwnershipClient {
	return &InsiderOwnershipClient{cacheDir: cacheDir}
}

func (c *InsiderOwnershipClient) FetchInsiderShares(ctx context.Context, symbols []string) map[string]domain.InsiderOwnership {
	targets := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		targets[strings.ToUpper(s)] = true
	}
	if len(targets) == 0 {
		return map[string]domain.InsiderOwnership{}
	}

	zipFiles, err := c.ensureCachedZips(ctx, insiderLookbackQuarters)
	if err != nil || len(zipFiles) == 0 {
		return map[string]domain.InsiderOwnership{}
	}
	return parseInsiderHoldings(zipFiles, targets)
}
