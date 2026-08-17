package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// RefreshExternalFundamentals trae sharesOutstanding (SEC EDGAR bulk),
// insider ownership (Form 3/4/5 bulk) y shortInterest/shortRatio (FINRA
// CSV) para todo el universo -- ninguno de los tres tiene un endpoint
// por-simbolo real, asi que procesar el universo completo de una pasada es
// tan barato como pedir un puñado de simbolos (el costo esta en
// descargar/decomprimir el archivo, cacheado en disco una vez por dia, no
// en cuantos simbolos se busquen adentro). floatShares REAL (restando
// holders institucionales 5%+) lo completa RefreshBeneficialOwners por
// separado en un loop continuo, ya que ese si es por-simbolo y no cabe en
// una sola pasada nocturna para 13k+ simbolos.
func RefreshExternalFundamentals(ctx context.Context, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, symbolsRepo out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) {
	tracked, err := symbolsRepo.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("external fundamentals refresh: failed to list tracked symbols")
		return
	}
	symbols := toSymbolStrings(tracked)
	start := time.Now()

	// DxLink primero: su "shares" es el dato en vivo (dxFeed lo actualiza
	// con las corporate actions), los filings de SEC EDGAR llegan tarde
	// tras diluciones/recompras (NIO -16% real medido). EDGAR sigue de
	// fallback para los simbolos que DxLink no cubre o trae en 0.
	profileShares := profile.FetchProfileShares(ctx, symbols)
	log.Info().Int("symbols", len(profileShares)).Msg("dxlink profile shares refresh finished")

	sharesOutstanding := edgar.FetchSharesOutstanding(ctx, symbols)
	log.Info().Int("symbols", len(sharesOutstanding)).Msg("sec edgar sharesOutstanding refresh finished")

	insiderData := insiders.FetchInsiderShares(ctx, symbols)
	log.Info().Int("symbols", len(insiderData)).Msg("sec insider ownership refresh finished")

	finraData := finra.DownloadLatest(ctx)
	log.Info().Int("symbols", len(finraData)).Msg("finra short interest refresh finished")

	updates := buildExternalFundamentals(symbols, profileShares, sharesOutstanding, insiderData, finraData)
	if err := fundamentalsRepo.UpsertExternalFundamentals(ctx, updates); err != nil {
		log.Error().Err(err).Msg("upserting external fundamentals refresh failed")
		return
	}
	log.Info().Int("symbols", len(updates)).Dur("elapsed", time.Since(start)).Msg("external fundamentals refresh finished")
}
