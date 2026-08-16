package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
)

const beneficialOwnersInterval = 5 * time.Minute

// StartBeneficialOwnersLoop completa floatShares real (sharesOutstanding -
// insiders - holders institucionales 5%+) en lotes chicos, de forma
// continua -- a diferencia de sharesOutstanding/insiders (archivos bulk,
// todo el universo de una pasada nocturna), holders 5%+ se piden por
// simbolo a la SEC, asi que 13k+ simbolos no caben en una sola ventana de
// mantenimiento. Un tick de 60 simbolos cada 5 min cubre el universo
// completo cada ~18h, de dia o de noche por igual -- SEC EDGAR no tiene
// nada que ver con el horario de mercado.
func StartBeneficialOwnersLoop(ctx context.Context, client out.BeneficialOwnersGateway, fundamentals out.FundamentalsRepository) {
	go func() {
		ticker := time.NewTicker(beneficialOwnersInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				catchup.RefreshBeneficialOwners(ctx, client, fundamentals)
			}
		}
	}()
}
