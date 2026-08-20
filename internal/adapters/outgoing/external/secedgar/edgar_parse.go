package secedgar

import (
	"archive/zip"
	"io"
	"strings"
)

// parseCompanyFactsZip hace UN solo pase sobre el bulk (~1.5GB) sacando
// tanto sharesOutstanding como la fecha de filing mas reciente -- las dos
// vienen del mismo JSON por CIK, pedirlas por separado significaria decodificar
// el archivo entero dos veces. totalTargets solo cuenta contra sharesOutstanding
// (el dato principal de esta pasada); filingDates se llena en el mismo loop
// sin condicionar el corte temprano.
func parseCompanyFactsZip(zipPath string, cikToSymbols map[string][]string, totalTargets int) (shares map[string]int64, filingDates map[string]string) {
	shares = make(map[string]int64)
	filingDates = make(map[string]string)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return shares, filingDates
	}
	defer r.Close()

	for _, entry := range r.File {
		if len(shares) >= totalTargets {
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

		facts := parseCompanyFacts(data)
		// El SO y la fecha de filing son de la entidad (CIK), no del ticker:
		// se aplican a todos los tickers del CIK (el principal y sus
		// warrants/preferentes). Antes se descartaba el CIK entero con mas
		// de un ticker, y el principal legitimo quedaba NULL -- confirmado
		// en vivo: T, SMCI, OPEN y BBD sin shares por compartir CIK con
		// TBB/SMCIP/OPENW/BBDO.
		for _, symbol := range matched {
			if facts.Shares != nil {
				shares[symbol] = *facts.Shares
			}
			if facts.LastFiled != "" {
				filingDates[symbol] = facts.LastFiled
			}
		}
	}
	return shares, filingDates
}

func extractCik(entryName string) string {
	if !strings.HasPrefix(entryName, "CIK") || !strings.HasSuffix(entryName, ".json") {
		return ""
	}
	return entryName[3 : len(entryName)-5]
}
