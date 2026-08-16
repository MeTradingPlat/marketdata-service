package secedgar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (l *TickerCikLookup) loadCachedOrDownload(ctx context.Context) (map[string]int, error) {
	if err := os.MkdirAll(l.cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating sec edgar cache dir: %w", err)
	}
	target := filepath.Join(l.cacheDir, "company_tickers_"+time.Now().Format("2006-01-02")+".json")
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return parseTickersFile(target)
	}

	dlCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := downloadToFile(dlCtx, tickersURL, target); err != nil {
		return nil, err
	}
	cleanupOldCacheFiles(l.cacheDir, "company_tickers_", target)
	return parseTickersFile(target)
}

func parseTickersFile(path string) (map[string]int, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cached tickers file: %w", err)
	}
	var raw map[string]struct {
		Ticker string `json:"ticker"`
		CikStr int    `json:"cik_str"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing tickers json: %w", err)
	}
	result := make(map[string]int, len(raw))
	for _, entry := range raw {
		if entry.Ticker != "" && entry.CikStr > 0 {
			result[strings.ToUpper(entry.Ticker)] = entry.CikStr
		}
	}
	return result, nil
}
