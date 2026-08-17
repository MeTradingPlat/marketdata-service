package main

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/finra"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/secedgar"
)

func main() {
	ctx := context.Background()
	symbols := []string{"AAPL", "MSFT", "GME"}

	tickerCik := secedgar.NewTickerCikLookup("/tmp/secedgar-cache")
	edgar := secedgar.NewEdgarClient(tickerCik, "/tmp/secedgar-cache")

	fmt.Println("=== SEC EDGAR sharesOutstanding ===")
	shares := edgar.FetchSharesOutstanding(ctx, symbols)
	for _, s := range symbols {
		fmt.Printf("%s: sharesOutstanding=%v\n", s, shares[s])
	}

	fmt.Println("=== SEC insider ownership ===")
	insiders := secedgar.NewInsiderOwnershipClient("/tmp/secedgar-cache")
	insiderData := insiders.FetchInsiderShares(ctx, symbols)
	for _, s := range symbols {
		o := insiderData[s]
		fmt.Printf("%s: insiderShares=%d, ownerCiks=%d\n", s, o.Shares, len(o.OwnerCiks))
	}

	fmt.Println("=== SEC beneficial owners (5%+) ===")
	beneficial := secedgar.NewBeneficialOwnersClient(tickerCik)
	for _, s := range symbols {
		excludeCiks := map[string]bool{}
		if o, ok := insiderData[s]; ok {
			excludeCiks = o.OwnerCiks
		}
		shares := beneficial.FetchBeneficialOwnerShares(ctx, s, excludeCiks)
		fmt.Printf("%s: beneficialOwnerShares=%d\n", s, shares)
	}

	fmt.Println("=== FINRA short interest ===")
	finraClient := finra.NewClient()
	finraData := finraClient.DownloadLatest(ctx)
	fmt.Printf("total symbols in FINRA file: %d\n", len(finraData))
	for _, s := range symbols {
		rec := finraData[s]
		fmt.Printf("%s: sharesShorted=%d, avgDailyVolume=%d, daysToCover=%.2f, settlement=%s\n",
			s, rec.SharesShorted, rec.AvgDailyVolume, rec.DaysToCover, rec.SettlementDate)
	}
}
