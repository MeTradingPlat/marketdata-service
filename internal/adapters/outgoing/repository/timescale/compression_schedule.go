package timescale

import (
	"context"
)

func (r *CandleRepository) SetCompressionScheduled(ctx context.Context, scheduled bool) error {
	_, err := r.pool.Exec(ctx, `SELECT alter_job(job_id, scheduled => $1) FROM timescaledb_information.jobs WHERE proc_name = 'policy_compression' AND hypertable_name = 'candles'`, scheduled)
	if err != nil {
		return err
	}
	if !scheduled {
		_, err = r.pool.Exec(ctx, `SELECT pg_cancel_backend(pid) FROM pg_stat_activity WHERE query LIKE 'CALL _timescaledb_functions.policy_compression%' AND pid <> pg_backend_pid()`)
		if err != nil {
			return err
		}
	}
	return nil
}
