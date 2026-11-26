package tastytrade

import (
	"context"
	"errors"
	"testing"
)

func TestOpenInterestFailure_CachesOnlyWhileContextIsAlive(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name   string
		ctx    context.Context
		cached bool
	}{
		{"alive", context.Background(), true},
		{"canceled", canceled, false},
	}
	for _, c := range cases {
		g := &Gateway{}
		g.openInterestCache.entries = map[string]openInterestEntry{}
		boom := errors.New("boom")
		if _, ok, err := g.openInterestFailure(c.ctx, "AAPL", "msg", boom); ok || !errors.Is(err, boom) {
			t.Fatalf("%s: got (%v, %v), want (false, boom)", c.name, ok, err)
		}
		if _, _, _, cached := g.openInterestCache.get("AAPL"); cached != c.cached {
			t.Fatalf("%s: cached = %v, want %v", c.name, cached, c.cached)
		}
	}
}
