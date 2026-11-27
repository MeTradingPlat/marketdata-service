package spool

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type FileSpool struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
	seq      uint64
}

func NewFileSpool(dir string, maxBytes int64) *FileSpool {
	return &FileSpool{dir: dir, maxBytes: maxBytes}
}

func (s *FileSpool) Write(candles []domain.Candle) error {
	if len(candles) == 0 {
		return nil
	}
	data, err := json.Marshal(toSpoolCandles(candles))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	if err := checkDirCap(s.dir, s.maxBytes, int64(len(data))); err != nil {
		return err
	}
	s.seq++
	name := fmt.Sprintf("%020d-%06d.json", time.Now().UnixNano(), s.seq)
	return writeAtomicFile(s.dir, name, data)
}
