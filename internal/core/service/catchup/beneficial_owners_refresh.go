package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const beneficialOwnersBatchSize = 60

// beneficialOwnersPacing: cada simbolo dispara varios requests a la SEC (1
// por submissions.json + uno por cada filing 13D/13G candidato, ~4-6 en
// promedio) -- sin pausa entre simbolos, un lote de 60 dispararia cientos
// de requests seguidos. Confirmado en Java: eso coincidio con
// marketdata-service dejando de responder por HTTP bajo carga real,
// consistente con la SEC degradando respuestas bajo esa raiz de requests.
const beneficialOwnersPacing = 400 * time.Millisecond

// RefreshBeneficialOwners completa floatShares real para un lote chico de
// simbolos por tick -- ver StartBeneficialOwnersLoop en cmd/api. Solo entra
// un simbolo que ya tenga sharesOutstanding+insiderShares conocidos (ver
// GetSymbolsDueForFloatRefresh), para no restar solo la mitad de las
// tenencias bloqueadas.
func RefreshBeneficialOwners(ctx context.Context, client out.BeneficialOwnersGateway, fundamentalsRepo out.FundamentalsRepository) {
	candidates, err := fundamentalsRepo.GetSymbolsDueForFloatRefresh(ctx, beneficialOwnersBatchSize)
	if err != nil {
		log.Error().Err(err).Msg("beneficial owners refresh: failed to select candidates")
		return
	}
	if len(candidates) == 0 {
		return
	}

	updates := make([]domain.Fundamentals, 0, len(candidates))
	for _, candidate := range candidates {
		excludeCiks := make(map[string]bool, len(candidate.InsiderCiks))
		for _, cik := range candidate.InsiderCiks {
			excludeCiks[cik] = true
		}

		beneficialShares := client.FetchBeneficialOwnerShares(ctx, candidate.Symbol, excludeCiks)
		realFloat := *candidate.SharesOutstanding - *candidate.InsiderShares - beneficialShares
		if realFloat < 0 {
			realFloat = 0
		}
		now := time.Now()
		updates = append(updates, domain.Fundamentals{Symbol: candidate.Symbol, FloatShares: &realFloat, FloatUpdatedAt: &now})

		select {
		case <-ctx.Done():
			return
		case <-time.After(beneficialOwnersPacing):
		}
	}

	if err := fundamentalsRepo.UpsertExternalFundamentals(ctx, updates); err != nil {
		log.Error().Err(err).Msg("upserting beneficial owners refresh failed")
		return
	}
	log.Info().Int("symbols", len(updates)).Msg("beneficial owners refresh finished")
}
