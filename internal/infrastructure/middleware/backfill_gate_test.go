package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
)

func newGateContext(e *echo.Echo, path string) echo.Context {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(path)
	return c
}

func TestBackfillGate_LightReadAlwaysPasses(t *testing.T) {
	var backfilling atomic.Bool
	backfilling.Store(true)
	e := echo.New()
	called := false
	handler := BackfillGate(&backfilling)(func(c echo.Context) error {
		called = true
		return nil
	})

	if err := handler(newGateContext(e, "/marketdata/quotes/rest")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the light-read handler to run even while backfilling")
	}
}

func TestBackfillGate_HeavyReadPassesWhenNotBackfilling(t *testing.T) {
	var backfilling atomic.Bool
	e := echo.New()
	called := false
	handler := BackfillGate(&backfilling)(func(c echo.Context) error {
		called = true
		return nil
	})

	if err := handler(newGateContext(e, "/marketdata/historical/AAPL")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the heavy-read handler to run when not backfilling")
	}
}

func TestBackfillGate_HeavyReadPassesWithFreeSlot(t *testing.T) {
	var backfilling atomic.Bool
	backfilling.Store(true)
	e := echo.New()
	called := false
	handler := BackfillGate(&backfilling)(func(c echo.Context) error {
		called = true
		return nil
	})

	if err := handler(newGateContext(e, "/marketdata/intraday/AAPL")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the heavy-read handler to run while a slot is free")
	}
}

func TestBackfillGate_HeavyReadThrottlesWhenSlotsFull(t *testing.T) {
	var backfilling atomic.Bool
	backfilling.Store(true)
	e := echo.New()

	occupiedSlot := make(chan struct{}, refillConcurrency)
	release := make(chan struct{})
	var wg sync.WaitGroup
	gate := BackfillGate(&backfilling)
	occupy := gate(func(c echo.Context) error {
		occupiedSlot <- struct{}{}
		<-release
		return nil
	})

	for i := 0; i < refillConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = occupy(newGateContext(e, "/marketdata/candles/AAPL/current"))
		}()
	}
	for i := 0; i < refillConcurrency; i++ {
		<-occupiedSlot
	}

	blocked := gate(func(c echo.Context) error {
		t.Fatal("handler should not run once slots are full")
		return nil
	})
	c := newGateContext(e, "/marketdata/candles/MSFT/current")
	if err := blocked(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Response().Status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", c.Response().Status, http.StatusServiceUnavailable)
	}

	close(release)
	wg.Wait()
}
