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
		if !ok || len(matched) != 1 {
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
			result[matched[0]] = *shares
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
