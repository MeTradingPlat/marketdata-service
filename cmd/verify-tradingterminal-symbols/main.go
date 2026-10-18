package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/pkg/verifybootstrap"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// probePacing replica beneficialOwnersPacing (400ms) en beneficial_owners_refresh.go
// para evitar saturar al proveedor con rafagas de pedidos historicos profundos.
const probePacing = 400 * time.Millisecond

type tickerItem struct {
	Symbol      string `json:"symbol"`
	CompanyName string `json:"companyName"`
}

type probeEntry struct {
	Original  string
	Converted string
}

type failureEntry struct {
	Original  string
	Converted string
	Error     string
}

// isNonAlnum detecta si el simbolo contiene al menos un caracter no alfanumerico ASCII (p.ej. puntos).
func isNonAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return true
		}
	}
	return false
}

// convertSymbol traduce sufijos .WS -> /WS y .U -> /U que TastyTrade maneja con barra (ver security_type.go).
func convertSymbol(sym string) (string, bool) {
	if strings.HasSuffix(sym, ".WS") {
		return strings.TrimSuffix(sym, ".WS") + "/WS", true
	}
	if strings.HasSuffix(sym, ".U") {
		return strings.TrimSuffix(sym, ".U") + "/U", true
	}
	return sym, false
}

// cmd/verify-tradingterminal-symbols descarga el catalogo estatico de TradingTerminal
// y evalua si TastyTrade reconoce y devuelve velas para simbolos con sufijos convertidos (.WS -> /WS, .U -> /U)
// y share-classes (.A/.B) frente a 5 simbolos de control.
func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	ctx := context.Background()

	// 1. Descarga y decodificacion del catalogo de tickers
	resp, err := http.Get("https://tt-static-files.s3.us-west-2.amazonaws.com/tickers.json.gz")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to download tickers catalog")
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		log.Fatal().Int("status", resp.StatusCode).Msg("unexpected HTTP status downloading tickers catalog")
	}

	// Si el transport http ya descomprimio la respuesta (resp.Uncompressed == true), no volvemos a pasar por gzip.NewReader.
	var bodyReader io.Reader = resp.Body
	var gz *gzip.Reader
	if !resp.Uncompressed {
		var gzErr error
		gz, gzErr = gzip.NewReader(resp.Body)
		if gzErr != nil {
			_ = resp.Body.Close()
			log.Fatal().Err(gzErr).Msg("failed to create gzip reader")
		}
		bodyReader = gz
	}

	var rawTickers []tickerItem
	decodeErr := json.NewDecoder(bodyReader).Decode(&rawTickers)
	if gz != nil {
		_ = gz.Close()
	}
	_ = resp.Body.Close()

	if decodeErr != nil {
		log.Fatal().Err(decodeErr).Msg("failed to decode tickers json")
	}

	// 2 y 3. Filtrado y agrupacion de simbolos con caracteres especiales
	var groupA []probeEntry
	var groupB []probeEntry
	for _, it := range rawTickers {
		if !isNonAlnum(it.Symbol) {
			continue
		}
		converted, isGroupA := convertSymbol(it.Symbol)
		entry := probeEntry{
			Original:  it.Symbol,
			Converted: converted,
		}
		if isGroupA {
			groupA = append(groupA, entry)
		} else {
			groupB = append(groupB, entry)
		}
	}

	// 4. Lista de sondeo: controles primero, luego grupo A y grupo B
	controls := []string{"AAPL", "MSFT", "NVDA", "GOOG", "AMZN"}
	probeList := make([]probeEntry, 0, len(controls)+len(groupA)+len(groupB))
	for _, c := range controls {
		probeList = append(probeList, probeEntry{
			Original:  c,
			Converted: c,
		})
	}
	probeList = append(probeList, groupA...)
	probeList = append(probeList, groupB...)

	// Bootstrap identico a cmd/verify-depth para inicializar gateway con CandlePool activo
	oauth := verifybootstrap.NewOAuth(configs.Load())
	qt := tastytrade.NewQuoteToken(oauth)
	if err := qt.Refresh(ctx); err != nil {
		log.Fatal().Err(err).Msg("quote token refresh failed")
	}
	connFactory := func(ctx context.Context) (*tastytrade.DxLinkConn, error) {
		conn := tastytrade.NewDxLinkConn(qt.DxlinkURL, qt.Token)
		if err := conn.Connect(ctx); err != nil {
			return nil, err
		}
		return conn, nil
	}
	pool := tastytrade.NewCandlePool(connFactory, 1)
	if err := pool.WarmUp(ctx); err != nil {
		log.Fatal().Err(err).Msg("candle pool warmup failed")
	}
	gateway := tastytrade.NewGateway(oauth, pool)

	// 5, 6 y 7. Ejecucion de sondeo con espaciado de 400ms
	var okCount int
	var errorCount int
	var failures []failureEntry

	for i, target := range probeList {
		if i > 0 {
			time.Sleep(probePacing)
		}

		candles, err := gateway.ProbeMaxDepth(ctx, target.Converted, domain.D1)
		if err != nil {
			errorCount++
			failures = append(failures, failureEntry{
				Original:  target.Original,
				Converted: target.Converted,
				Error:     err.Error(),
			})
			log.Error().
				Str("original", target.Original).
				Str("converted", target.Converted).
				Str("result", "ERROR").
				Err(err).
				Msg("probe failed")
			continue
		}

		okCount++
		event := log.Info().
			Str("original", target.Original).
			Str("converted", target.Converted).
			Str("result", "OK").
			Int("candles", len(candles))
		if len(candles) > 0 {
			oldest := candles[0].Timestamp.UTC().Format(time.RFC3339)
			newest := candles[len(candles)-1].Timestamp.UTC().Format(time.RFC3339)
			event.Str("oldest", oldest).Str("newest", newest)
		}
		event.Msg("probe completed")
	}

	// 8. Resumen final
	log.Info().
		Int("total_probed", len(probeList)).
		Int("ok_count", okCount).
		Int("error_count", errorCount).
		Msg("probe summary completed")

	fmt.Printf("\n=== PROBE SUMMARY ===\n")
	fmt.Printf("Total probed: %d\n", len(probeList))
	fmt.Printf("OK: %d\n", okCount)
	fmt.Printf("ERROR: %d\n", errorCount)

	if len(failures) > 0 {
		fmt.Printf("\n=== FAILED ENTRIES (%d) ===\n", len(failures))
		for _, f := range failures {
			fmt.Printf("Original: %-10s | Converted: %-10s | Error: %s\n", f.Original, f.Converted, f.Error)
		}
	} else {
		fmt.Printf("\nNo failed entries.\n")
	}
}
