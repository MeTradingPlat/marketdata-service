package spool_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFileSpool_CapTempAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".tmp-stale"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000001-000001.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := spool.NewFileSpool(dir, 500)
	ids, err := s.Pending()
	if err != nil || len(ids) != 1 {
		t.Fatalf("Pending = (%v, %v), want 1", ids, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tmp-stale")); !os.IsNotExist(err) {
		t.Fatalf("expected stale temp file removed, stat err = %v", err)
	}
	if _, err := s.Read(ids[0]); err == nil {
		t.Fatal("expected error reading corrupt file")
	}
	if _, err := os.Stat(filepath.Join(dir, ids[0]+".corrupt")); err != nil {
		t.Fatalf("expected .corrupt rename, stat err = %v", err)
	}
	if _, err := s.Read("missing.json"); err == nil {
		t.Fatal("expected error reading missing file")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.json.corrupt")); !os.IsNotExist(err) {
		t.Fatalf("expected missing file not renamed, stat err = %v", err)
	}

	if err := s.Write([]domain.Candle{{Symbol: "AAPL"}, {Symbol: "MSFT"}, {Symbol: "NVDA"}, {Symbol: "TSLA"}}); err == nil {
		t.Fatal("expected error when exceeding maxBytes cap")
	}
}
