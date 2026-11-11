package secedgar

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func newestCachedCompanyFactsZip(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "companyfacts_") && strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tmp") {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})
	return filepath.Join(dir, matches[0]), true
}
