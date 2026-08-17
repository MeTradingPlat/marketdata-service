package secedgar

import (
	"archive/zip"
	"io"
	"strings"
)

func parseCompanyFactsZip(zipPath string, cikToSymbols map[string][]string, totalTargets int) map[string]int64 {
	result := make(map[string]int64)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return result
	}
	defer r.Close()

	for _, entry := range r.File {
		if len(result) >= totalTargets {
			break
		}
		matched, ok := cikToSymbols[extractCik(entry.Name)]
		if !ok {
			continue
		}

		f, err := entry.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			continue
		}

		if shares := parseSharesOutstanding(data); shares != nil {
			// El SO es de la entidad (CIK), no del ticker: se aplica a todos
			// los tickers del CIK (el principal y sus warrants/preferentes).
			// Antes se descartaba el CIK entero con mas de un ticker, y el
			// principal legitimo quedaba NULL -- confirmado en vivo: T, SMCI,
			// OPEN y BBD sin shares por compartir CIK con TBB/SMCIP/OPENW/BBDO.
			for _, symbol := range matched {
				result[symbol] = *shares
			}
		}
	}
	return result
}

func extractCik(entryName string) string {
	if !strings.HasPrefix(entryName, "CIK") || !strings.HasSuffix(entryName, ".json") {
		return ""
	}
	return entryName[3 : len(entryName)-5]
}
