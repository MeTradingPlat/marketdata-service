package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	equitiesPerPage  = 1000
	equitiesMaxPages = 100

	// equitiesPageThrottle espacia las peticiones de paginas del universo
	// completo -- mismo throttle preventivo que ya usaba el servicio Java
	// contra el mismo endpoint (ver EquitiesUniverseProvider), evita golpear
	// el rate limit de TastyTrade en una corrida de ~14 paginas seguidas.
	equitiesPageThrottle = 500 * time.Millisecond
)

// allowedMarkets son los MIC codes de los mercados que la app realmente
// soporta -- mismo conjunto que EnumMercado.java (NYSE/NASDAQ/AMEX/ETF/OTC).
// El endpoint de TastyTrade reporta "activo" cualquier cosa que cotice, sin
// este filtro el universo se contamina con mercados que no manejamos.
var allowedMarkets = map[string]struct{}{
	"XNAS": {}, "XNYS": {}, "XASE": {}, "ARCX": {}, "BATS": {}, "OTC": {},
}

type equitiesResponse struct {
	Data struct {
		Items []struct {
			Symbol       string `json:"symbol"`
			Description  string `json:"description"`
			ListedMarket string `json:"listed-market"`
			IsEtf        bool   `json:"is-etf"`
		} `json:"items"`
	} `json:"data"`
}

func (g *Gateway) ActiveSymbols(ctx context.Context) ([]domain.Symbol, error) {
	var all []domain.Symbol
	for page := 0; page < equitiesMaxPages; page++ {
		if page > 0 {
			time.Sleep(equitiesPageThrottle)
		}
		batch, rawCount, err := g.fetchEquitiesPage(ctx, page, equitiesPerPage)
		if err != nil {
			return nil, err
		}
		if rawCount == 0 {
			break
		}
		all = append(all, batch...)
		// rawCount, no len(batch): el filtro de mercado puede tirar una
		// pagina llena por debajo de equitiesPerPage sin que sea realmente
		// la ultima -- cortar por el conteo filtrado paraba la paginacion
		// antes de tiempo.
		if rawCount < equitiesPerPage {
			break
		}
	}
	return all, nil
}

func (g *Gateway) fetchEquitiesPage(ctx context.Context, page, perPage int) ([]domain.Symbol, int, error) {
	url := fmt.Sprintf("%s/instruments/equities/active?per-page=%d&page-offset=%d", g.oauth.cfg.BaseURL, perPage, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building equities request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("calling equities endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("equities endpoint returned status %d", resp.StatusCode)
	}

	var er equitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, 0, fmt.Errorf("decoding equities response: %w", err)
	}

	symbols := make([]domain.Symbol, 0, len(er.Data.Items))
	for _, item := range er.Data.Items {
		if _, ok := allowedMarkets[item.ListedMarket]; !ok {
			continue
		}
		if isTestSymbol(item.Symbol, item.Description) {
			continue
		}
		symbols = append(symbols, domain.Symbol{Symbol: item.Symbol, Description: item.Description, Market: item.ListedMarket, IsEtf: item.IsEtf})
	}
	return symbols, len(er.Data.Items), nil
}

// isTestSymbol identifica los tickers de prueba que los exchanges publican
// para verificar que el feed funciona (ATEST/*, CTEST, NTEST/*, PTEST/*,
// ZAZZT/ZBZZT/ZCZZT "tick pilot", ZXYZ/A "NASDAQ SYMBOLOGY TST") -- nunca
// cotizan nada real, mismo criterio que ya usaba classifySecurityType() en
// el servicio Java (confirmado en vivo contra /instruments/equities).
// Al no incluirlos en ActiveSymbols(), reconcileUniverse() los desactiva
// solos si ya estaban rastreados, sin logica nueva de por medio.
func isTestSymbol(symbol, description string) bool {
	if strings.Contains(symbol, "TEST") {
		return true
	}
	d := strings.ToLower(description)
	return strings.Contains(d, "tick pilot") || strings.Contains(d, "symbology tst")
}
