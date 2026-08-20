package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
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
func RefreshExternalFundamentals(ctx context.Context, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, symbolsRepo out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) error {
	tracked, err := symbolsRepo.Tracked(ctx)
	if err != nil {
		return fmt.Errorf("listing tracked symbols: %w", err)
	}
	symbols := toSymbolStrings(tracked)
	start := time.Now()

	// DxLink primero: su "shares" es el dato en vivo (dxFeed lo actualiza
	// con las corporate actions), los filings de SEC EDGAR llegan tarde
	// tras diluciones/recompras (NIO -16% real medido). EDGAR sigue de
	// fallback para los simbolos que DxLink no cubre o trae en 0.
	profileShares := profile.FetchProfileShares(ctx, symbols)
	log.Info().Int("symbols", len(profileShares)).Msg("dxlink profile shares refresh finished")

	sharesOutstanding, filingDates := edgar.FetchCompanyFacts(ctx, symbols)
	log.Info().Int("symbols", len(sharesOutstanding)).Msg("sec edgar sharesOutstanding refresh finished")

	insiderData := insiders.FetchInsiderShares(ctx, symbols)
	log.Info().Int("symbols", len(insiderData)).Msg("sec insider ownership refresh finished")

	finraData, err := finra.DownloadLatest(ctx)
	if err != nil {
		return fmt.Errorf("finra short interest: %w", err)
	}
	log.Info().Int("symbols", len(finraData)).Msg("finra short interest refresh finished")

	updates := buildExternalFundamentals(symbols, profileShares, sharesOutstanding, insiderData, finraData)
	if err := fundamentalsRepo.UpsertExternalFundamentals(ctx, updates); err != nil {
		return fmt.Errorf("upserting external fundamentals batch: %w", err)
	}
	log.Info().Int("symbols", len(updates)).Dur("elapsed", time.Since(start)).Msg("external fundamentals refresh finished")

	if err := fillEarningsFromSecFilings(ctx, fundamentalsRepo, filingDates); err != nil {
		log.Error().Err(err).Msg("sec edgar earnings fallback failed")
	}
	return nil
}

// fillEarningsFromSecFilings usa la fecha del ultimo filing (10-Q/10-K) de
// SEC EDGAR como proxy de "ultimo reporte" SOLO para los simbolos que
// RefreshEarningsHistory (TastyTrade, que corre antes en el ciclo -- ver
// universe_cycle.go) no pudo resolver esta misma noche -- re-consulta el
// mismo criterio de "vencido o nunca buscado" ya con los resultados de
// TastyTrade aplicados, para nunca pisar un dato mejor con este proxy mas
// debil (TastyTrade tiene prioridad). No fabrica una prediccion de proxima
// fecha desde esto -- ver el comentario de RefreshEarningsHistory sobre por
// que preferimos no predecir antes que predecir mal.
func fillEarningsFromSecFilings(ctx context.Context, fundamentalsRepo out.FundamentalsRepository, filingDates map[string]string) error {
	stale, err := fundamentalsRepo.GetSymbolsWithStaleEarnings(ctx)
	if err != nil {
		return fmt.Errorf("selecting still-stale symbols: %w", err)
	}
	if len(stale) == 0 {
		return nil
	}

	var updates []domain.Fundamentals
	for _, symbol := range stale {
		if filed, ok := filingDates[symbol]; ok && filed != "" {
			updates = append(updates, domain.Fundamentals{Symbol: symbol, OccurredDate: filed})
		}
	}
	if len(updates) == 0 {
		return nil
	}
	if err := fundamentalsRepo.UpsertEarningsHistory(ctx, updates); err != nil {
		return fmt.Errorf("upserting sec edgar earnings fallback: %w", err)
	}
	log.Info().Int("symbols", len(updates)).Msg("sec edgar earnings fallback finished")
	return nil
}
