package spool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (s *FileSpool) Pending() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return collectPending(s.dir, entries), nil
}

func collectPending(dir string, entries []os.DirEntry) []string {
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".tmp-") {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		if strings.HasSuffix(name, ".json") {
			ids = append(ids, name)
		}
	}
	return ids
}

func (s *FileSpool) Read(id string) ([]domain.Candle, error) {
	path := filepath.Join(s.dir, filepath.Base(id))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var candles []domain.Candle
	if err := json.Unmarshal(data, &candles); err != nil {
		_ = os.Rename(path, path+".corrupt")
		return nil, err
	}
	return candles, nil
}

func (s *FileSpool) Remove(id string) error {
	return os.Remove(filepath.Join(s.dir, filepath.Base(id)))
}
