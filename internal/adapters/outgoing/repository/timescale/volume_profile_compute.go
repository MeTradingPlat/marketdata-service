package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const volumeProfileLookback = 35 * 24 * time.Hour

const volumeProfileSymbolIDsSQL = `SELECT symbol_id, symbol FROM tracked_symbols WHERE symbol = ANY($1)`

const volumeProfileM1SQL = `
	SELECT symbol_id, ts, volume
	FROM candles
	WHERE timeframe = 'M1' AND symbol_id = ANY($1::int[]) AND ts >= $2 AND ts < $3`

func (r *VolumeProfileRepository) ComputeProfiles(ctx context.Context, symbols []string, before time.Time) (map[string]domain.VolumeProfile, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading New York timezone: %w", err)
	}
	nameByID, ids, err := r.symbolIDs(ctx, symbols)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, volumeProfileM1SQL, ids, before.Add(-volumeProfileLookback), before)
	if err != nil {
		return nil, fmt.Errorf("querying M1 volumes for %d symbols: %w", len(ids), err)
	}
	defer rows.Close()

	bySymbol := make(map[int32]*domain.SessionVolumes, len(ids))
	for rows.Next() {
		var id int32
		var ts time.Time
		var volume int64
		if err := rows.Scan(&id, &ts, &volume); err != nil {
			return nil, fmt.Errorf("scanning M1 volume row: %w", err)
		}
		session, slot, ok := domain.SlotOf(ts, loc)
		if !ok {
			continue
		}
		volumes, exists := bySymbol[id]
		if !exists {
			volumes = domain.NewSessionVolumes()
			bySymbol[id] = volumes
		}
		volumes.Add(session, slot, volume)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	profiles := make(map[string]domain.VolumeProfile, len(bySymbol))
	for id, volumes := range bySymbol {
		sessions, cumulative := volumes.Profile(domain.VolumeProfileSessions)
		if sessions > 0 {
			symbol := nameByID[id]
			profiles[symbol] = domain.VolumeProfile{Symbol: symbol, Sessions: sessions, Cumulative: cumulative}
		}
	}
	return profiles, nil
}

func (r *VolumeProfileRepository) symbolIDs(ctx context.Context, symbols []string) (map[int32]string, []int32, error) {
	rows, err := r.pool.Query(ctx, volumeProfileSymbolIDsSQL, symbols)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving symbol ids: %w", err)
	}
	defer rows.Close()
	nameByID := make(map[int32]string, len(symbols))
	ids := make([]int32, 0, len(symbols))
	for rows.Next() {
		var id int32
		var symbol string
		if err := rows.Scan(&id, &symbol); err != nil {
			return nil, nil, fmt.Errorf("scanning symbol id: %w", err)
		}
		nameByID[id] = symbol
		ids = append(ids, id)
	}
	return nameByID, ids, rows.Err()
}
