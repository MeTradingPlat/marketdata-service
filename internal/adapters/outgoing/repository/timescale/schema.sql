CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS tracked_symbols (
    symbol_id     SERIAL PRIMARY KEY,
    symbol        VARCHAR(20) NOT NULL UNIQUE,
    market        VARCHAR(10) NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS candles (
    symbol_id   INT NOT NULL REFERENCES tracked_symbols(symbol_id),
    timeframe   VARCHAR(6) NOT NULL,
    ts          TIMESTAMPTZ NOT NULL,
    open        NUMERIC(18,6) NOT NULL,
    high        NUMERIC(18,6) NOT NULL,
    low         NUMERIC(18,6) NOT NULL,
    close       NUMERIC(18,6) NOT NULL,
    volume      BIGINT NOT NULL DEFAULT 0,
    trade_count INT,
    vwap        NUMERIC(18,6),
    source      VARCHAR(16) NOT NULL DEFAULT 'tastytrade',
    PRIMARY KEY (symbol_id, timeframe, ts)
);

CREATE INDEX IF NOT EXISTS idx_candles_symbol_tf ON candles (symbol_id, timeframe);

-- Mismo patron probado en historical-data-service: hypertable de 7 dias,
-- comprimida por serie (symbol_id+timeframe), con buffer de 7 dias antes de
-- comprimir para que los UPSERTs de la vela recien cerrada sigan aceptados.
SELECT create_hypertable('candles', 'ts', chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

ALTER TABLE candles SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'symbol_id, timeframe',
    timescaledb.compress_orderby = 'ts DESC'
);

SELECT add_compression_policy('candles', INTERVAL '7 days', if_not_exists => TRUE);

-- Mismo tuning de historical-data-service: evita la pasada de vacuum de
-- 30+ min vista en produccion con esta tabla bajo escritura constante.
ALTER TABLE candles SET (
    autovacuum_vacuum_scale_factor = 0.02,
    autovacuum_analyze_scale_factor = 0.02,
    autovacuum_vacuum_cost_limit = 1000
);
