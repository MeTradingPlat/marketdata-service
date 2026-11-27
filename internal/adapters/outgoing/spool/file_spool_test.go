package spool_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFileSpool_WritePendingReadRemove(t *testing.T) {
	dir := t.TempDir() + "/missing"
	s := spool.NewFileSpool(dir, 10*1024*1024)

	ids, err := s.Pending()
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty Pending = (%v, %v), want empty nil", ids, err)
	}

	b1 := []domain.Candle{{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: time.Unix(100, 0).UTC(), Close: 1}}
	b2 := []domain.Candle{{Symbol: "MSFT", Timeframe: domain.M1, Timestamp: time.Unix(200, 0).UTC(), Close: 2}}
	if err := s.Write(b1); err != nil {
		t.Fatalf("Write b1: %v", err)
	}
	if err := s.Write(b2); err != nil {
		t.Fatalf("Write b2: %v", err)
	}

	ids, err = s.Pending()
	if err != nil || len(ids) != 2 {
		t.Fatalf("Pending = (%v, %v), want 2 ids", ids, err)
	}
	got1, err := s.Read(ids[0])
	if err != nil || len(got1) != 1 || got1[0].Symbol != "AAPL" {
		t.Fatalf("Read first = (%+v, %v), want AAPL", got1, err)
	}
	got2, err := s.Read(ids[1])
	if err != nil || len(got2) != 1 || got2[0].Symbol != "MSFT" {
		t.Fatalf("Read second = (%+v, %v), want MSFT", got2, err)
	}
	if err := s.Remove(ids[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	ids, err = s.Pending()
	if err != nil || len(ids) != 1 {
		t.Fatalf("Pending after Remove = (%v, %v), want 1", ids, err)
	}
}

func TestFileSpool_PreservesObservedAt(t *testing.T) {
	dir := t.TempDir()
	s := spool.NewFileSpool(dir, 10*1024*1024)
	obs := time.Date(2026, 11, 27, 15, 0, 5, 123000000, time.UTC)
	batch := []domain.Candle{{
		Symbol:     "AAPL",
		Timeframe:  domain.M1,
		Timestamp:  time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC),
		Close:      150,
		ObservedAt: obs,
	}}
	if err := s.Write(batch); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ids, err := s.Pending()
	if err != nil || len(ids) != 1 {
		t.Fatalf("Pending = (%v, %v), want 1", ids, err)
	}
	got, err := s.Read(ids[0])
	if err != nil || len(got) != 1 {
		t.Fatalf("Read = (%+v, %v), want 1 candle", got, err)
	}
	if !got[0].ObservedAt.Equal(obs) {
		t.Fatalf("ObservedAt = %v, want %v", got[0].ObservedAt, obs)
	}
}

func TestFileSpool_ReadsLegacyFileWithoutObservedAt(t *testing.T) {
	dir := t.TempDir()
	s := spool.NewFileSpool(dir, 10*1024*1024)
	legacy := `[{"symbol":"AAPL","timeframe":"M1","timestamp":"2026-11-27T15:00:00Z","open":100,"high":101,"low":99,"close":100.5,"volume":10,"tradeCount":1,"vwap":100.2,"source":"tastytrade"}]`
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000001-000001.json"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ids, err := s.Pending()
	if err != nil || len(ids) != 1 {
		t.Fatalf("Pending = (%v, %v), want 1", ids, err)
	}
	got, err := s.Read(ids[0])
	if err != nil || len(got) != 1 {
		t.Fatalf("Read = (%+v, %v), want 1 candle", got, err)
	}
	if got[0].Symbol != "AAPL" || !got[0].ObservedAt.IsZero() {
		t.Fatalf("got = %+v, want AAPL with zero ObservedAt", got[0])
	}
}
