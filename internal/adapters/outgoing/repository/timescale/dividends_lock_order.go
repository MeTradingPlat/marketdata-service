package timescale

import (
	"maps"
	"slices"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func bySymbol(fundamentals []domain.Fundamentals) []domain.Fundamentals {
	sorted := slices.Clone(fundamentals)
	slices.SortFunc(sorted, func(a, b domain.Fundamentals) int { return strings.Compare(a.Symbol, b.Symbol) })
	return sorted
}

func lockOrder[V any](values map[string]V, attemptedOnly []string) []string {
	symbols := append(slices.Collect(maps.Keys(values)), attemptedOnly...)
	slices.Sort(symbols)
	return slices.Compact(symbols)
}
