package timescale

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func bar(symbol string) []domain.Candle {
	return []domain.Candle{{Symbol: symbol}}
}

func TestSeriesLookbackTiers_UnaVentanaChicaPruebaHoyAntesDeLosDiezDias(t *testing.T) {
	got := seriesLookbackTiers(1, time.Minute)

	want := []time.Duration{24 * time.Hour, 10 * 24 * time.Hour}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSeriesLookbackTiers_D1YPedidosGrandesUsanUnaSolaVentana(t *testing.T) {
	for name, tiers := range map[string][]time.Duration{
		"D1":        seriesLookbackTiers(21, 24*time.Hour),
		"M1 grande": seriesLookbackTiers(500, time.Minute),
		"M5 grande": seriesLookbackTiers(1000, 5*time.Minute),
	} {
		if len(tiers) != 1 {
			t.Errorf("%s: want a single window, got %v", name, tiers)
		}
	}
}

func TestGetSeriesTiered_SoloLosSimbolosSinDatoPasanALaVentanaAncha(t *testing.T) {
	var calls [][]string
	query := func(_ context.Context, symbols []string, window time.Duration) (map[string][]domain.Candle, error) {
		calls = append(calls, symbols)
		if window == 24*time.Hour {
			return map[string][]domain.Candle{"AAA": bar("AAA"), "BBB": bar("BBB")}, nil
		}
		return map[string][]domain.Candle{"CCC": bar("CCC")}, nil
	}

	got, err := getSeriesTiered(context.Background(), []string{"AAA", "BBB", "CCC", "DDD"}, 1,
		[]time.Duration{24 * time.Hour, 10 * 24 * time.Hour}, query)

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls[1], []string{"CCC", "DDD"}) {
		t.Fatalf("second window must only query the missing symbols, got %v", calls[1])
	}
	if len(got) != 3 || got["CCC"] == nil || got["DDD"] != nil {
		t.Fatalf("unexpected merge: %v", got)
	}
}

func TestGetSeriesTiered_SiLaVentanaChicaLoCubreTodoNoConsultaLaAncha(t *testing.T) {
	calls := 0
	query := func(_ context.Context, symbols []string, _ time.Duration) (map[string][]domain.Candle, error) {
		calls++
		return map[string][]domain.Candle{"AAA": bar("AAA")}, nil
	}

	_, err := getSeriesTiered(context.Background(), []string{"AAA"}, 1,
		[]time.Duration{24 * time.Hour, 10 * 24 * time.Hour}, query)

	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want a single query", err, calls)
	}
}

func TestGetSeriesTiered_UnErrorDeUnaVentanaSePropaga(t *testing.T) {
	boom := errors.New("boom")
	query := func(context.Context, []string, time.Duration) (map[string][]domain.Candle, error) {
		return nil, boom
	}

	_, err := getSeriesTiered(context.Background(), []string{"AAA"}, 1, []time.Duration{time.Hour}, query)

	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
}
