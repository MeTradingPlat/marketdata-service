package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// openInterestCacheTTL: el open interest se recalcula una vez por dia en
// OCC -- 6h de cache alcanza para no repetir el doble REST por simbolo en
// cada poll del frontend.
const openInterestCacheTTL = 6 * time.Hour

type openInterestEntry struct {
	expires time.Time
	value   float64
	found   bool
}

type openInterestCache struct {
	mu      sync.Mutex
	entries map[string]openInterestEntry
}

func (c *openInterestCache) get(symbol string) (float64, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[symbol]
	if !ok {
		return 0, false, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, symbol)
		return 0, false, false
	}
	return entry.value, entry.found, true
}

func (c *openInterestCache) put(symbol string, value float64, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[symbol] = openInterestEntry{expires: time.Now().Add(openInterestCacheTTL), value: value, found: found}
}

type expirationsResponse struct {
	Data struct {
		Expirations []struct {
			ExpirationDate string `json:"expiration-date"`
		} `json:"expirations"`
	} `json:"data"`
}

type optionChainResponse struct {
	Data struct {
		Items []struct {
			OpenInterest any `json:"open-interest"`
		} `json:"items"`
	} `json:"data"`
}

// OpenInterest suma el open interest del vencimiento mensual mas cercano
// (convencion de la industria para "Open Interest" de un subyacente) --
// dos REST por simbolo, cacheado 6h. Un simbolo sin opciones (o con la
// cadena vacia) devuelve (0, false) SIN error: no es un fallo del
// servicio, es un dato que no existe -- el detalle lo omite y el frontend
// muestra N/A (misma disciplina null-vs-0 del resto de fundamentales).
func (g *Gateway) OpenInterest(ctx context.Context, symbol string) (float64, bool) {
	if value, found, ok := g.openInterestCache.get(symbol); ok {
		return value, found
	}

	expiration, err := g.frontMonthlyExpiration(ctx, symbol)
	if err != nil || expiration == "" {
		if err != nil {
			log.Debug().Err(err).Str("symbol", symbol).Msg("open interest: no option chain available")
		}
		g.openInterestCache.put(symbol, 0, false)
		return 0, false
	}

	total, err := g.sumOpenInterest(ctx, symbol, expiration)
	if err != nil {
		log.Debug().Err(err).Str("symbol", symbol).Msg("open interest: failed to load front month chain")
		g.openInterestCache.put(symbol, 0, false)
		return 0, false
	}
	g.openInterestCache.put(symbol, total, total > 0)
	return total, total > 0
}

func (g *Gateway) frontMonthlyExpiration(ctx context.Context, symbol string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.oauth.cfg.BaseURL+"/option-chains/"+symbol, nil)
	if err != nil {
		return "", fmt.Errorf("building option-chains request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling option-chains: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("option-chains returned status %d", resp.StatusCode)
	}

	var er expirationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return "", fmt.Errorf("decoding option-chains: %w", err)
	}

	now := time.Now()
	for _, e := range er.Data.Expirations {
		t, err := time.Parse("2006-01-02", e.ExpirationDate)
		if err != nil {
			continue
		}
		if t.After(now) {
			return e.ExpirationDate, nil
		}
	}
	return "", nil
}

func (g *Gateway) sumOpenInterest(ctx context.Context, symbol, expiration string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.oauth.cfg.BaseURL+"/option-chains/"+symbol+"/"+expiration, nil)
	if err != nil {
		return 0, fmt.Errorf("building option chain request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("calling option chain: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("option chain returned status %d", resp.StatusCode)
	}

	var cr optionChainResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return 0, fmt.Errorf("decoding option chain: %w", err)
	}

	var total float64
	for _, item := range cr.Data.Items {
		total += openInterestValue(item.OpenInterest)
	}
	return total, nil
}

// openInterestValue tolera el numero crudo o en string -- TastyTrade
// devuelve los campos numericos como strings en varias respuestas (mismo
// patrón que el parseo de eps en earnings_reports.go).
func openInterestValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f
		}
	}
	return 0
}
