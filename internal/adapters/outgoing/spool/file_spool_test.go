package spool_test

import (
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
