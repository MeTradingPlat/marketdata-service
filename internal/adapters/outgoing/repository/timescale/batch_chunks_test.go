package timescale

import (
	"reflect"
	"testing"
)

func TestChunkSymbols(t *testing.T) {
	cases := []struct {
		name    string
		symbols []string
		size    int
		want    [][]string
	}{
		{"empty", nil, 2, [][]string{nil}},
		{"under size", []string{"AAPL", "MSFT"}, 5, [][]string{{"AAPL", "MSFT"}}},
		{"exact multiple", []string{"A", "B", "C", "D"}, 2, [][]string{{"A", "B"}, {"C", "D"}}},
		{"remainder", []string{"A", "B", "C", "D", "E"}, 2, [][]string{{"A", "B"}, {"C", "D"}, {"E"}}},
		{"size zero returns single chunk", []string{"A", "B"}, 0, [][]string{{"A", "B"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chunkSymbols(c.symbols, c.size)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("chunkSymbols(%v, %d) = %v, want %v", c.symbols, c.size, got, c.want)
			}
		})
	}
}
