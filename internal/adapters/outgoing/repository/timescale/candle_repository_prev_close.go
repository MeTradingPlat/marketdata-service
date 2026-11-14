package timescale

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// prevSessionCloseDays son cuantos dias de calendario hacia atras se buscan
// ventanas de cierre regular -- 10 cubre cualquier racha sin mercado
// (fines de semana + feriados encadenados, ej. Thanksgiving + fin de
// semana) con ventanas de 4 minutos por dia: 10 busquedas por indice
// (symbol_id, timeframe, ts), no un scan de toda la historia.
const prevSessionCloseDays = 10

// GetPreviousSessionClose: ver el puerto (ports/out/repositories.go). Las
// ventanas se calculan en America/New_York (la subasta de cierre cae a las
// 16:00 ET tambien en horario de verano) y se mandan como parametros --
// el planner de Postgres puede usar el indice por cada rama del OR, igual
// que el resto de consultas de este repositorio.
func (r *CandleRepository) GetPreviousSessionClose(ctx context.Context, symbol string, before time.Time) (*float64, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading America/New_York location: %w", err)
	}
	dayStart := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, loc)

	args := []any{symbol, before}
	windows := make([]string, 0, prevSessionCloseDays)
	for d := 1; d <= prevSessionCloseDays; d++ {
		day := dayStart.AddDate(0, 0, -d)
		from := day.Add(15*time.Hour + 58*time.Minute)
		// to = 16:01 ET: la ts=16:00 es la subasta de cierre, la 16:01 ya es after-hours.
		to := day.Add(16*time.Hour + time.Minute)
		windows = append(windows, fmt.Sprintf("($%d::timestamptz <= c.ts AND c.ts < $%d::timestamptz)", len(args)+1, len(args)+2))
		args = append(args, from, to)
	}

	query := fmt.Sprintf(`
		SELECT c.close
		FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
		WHERE s.symbol = $1 AND c.timeframe = 'M1' AND c.ts < $2
			AND (%s)
		ORDER BY c.ts DESC LIMIT 1`, strings.Join(windows, " OR "))

	var close float64
	err = r.pool.QueryRow(ctx, query, args...).Scan(&close)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying previous session close for %s: %w", symbol, err)
	}
	return &close, nil
}

// previousSessionCloseWindowSQL busca la subasta de UN dia para el lote entero; una ventana angosta por llamada solo toca 1-2 chunks.
const previousSessionCloseWindowSQL = `
	SELECT DISTINCT ON (s.symbol) s.symbol, c.close
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE s.symbol = ANY($1) AND c.timeframe = 'M1' AND c.ts >= $2 AND c.ts < $3
	ORDER BY s.symbol, c.ts DESC
`

// GetPreviousSessionCloseBatch es GetPreviousSessionClose para un lote --
// itera las mismas ventanas [15:58,16:01) ET dia por dia (ver
// prevSessionCloseDays), pero con una query angosta por dia en vez de un
// solo OR gigante (ver previousSessionCloseWindowSQL) y se corta apenas
// todos los simbolos pedidos ya resolvieron -- en la practica casi todos
// caen en la primera vuelta (el dia habil mas reciente), asi que el caso
// comun paga una sola query angosta, no diez.
func (r *CandleRepository) GetPreviousSessionCloseBatch(ctx context.Context, symbols []string, before time.Time) (map[string]float64, error) {
	result := make(map[string]float64, len(symbols))
	if len(symbols) == 0 {
		return result, nil
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading America/New_York location: %w", err)
	}
	dayStart := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, loc)

	pending := make([]string, len(symbols))
	copy(pending, symbols)
	for d := 1; d <= prevSessionCloseDays && len(pending) > 0; d++ {
		day := dayStart.AddDate(0, 0, -d)
		from := day.Add(15*time.Hour + 58*time.Minute)
		to := day.Add(16*time.Hour + time.Minute)

		resolved := make(map[string]struct{}, len(pending))
		for _, batch := range chunkSymbols(pending, universeBatchChunkSize) {
			rows, err := r.snapshotPool.Query(ctx, previousSessionCloseWindowSQL, batch, from, to)
			if err != nil {
				return nil, fmt.Errorf("querying previous session close window (day -%d) for %d symbols: %w", d, len(batch), err)
			}
			for rows.Next() {
				var symbol string
				var close float64
				if err := rows.Scan(&symbol, &close); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scanning previous session close window row: %w", err)
				}
				result[symbol] = close
				resolved[symbol] = struct{}{}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, fmt.Errorf("iterating previous session close window rows: %w", err)
			}
			rows.Close()
		}

		if len(resolved) == 0 {
			continue
		}
		next := pending[:0]
		for _, sym := range pending {
			if _, ok := resolved[sym]; !ok {
				next = append(next, sym)
			}
		}
		pending = next
	}
	return result, nil
}
