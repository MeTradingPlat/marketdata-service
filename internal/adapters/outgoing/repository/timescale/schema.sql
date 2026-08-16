CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS tracked_symbols (
    symbol_id     SERIAL PRIMARY KEY,
    symbol        VARCHAR(20) NOT NULL UNIQUE,
    market        VARCHAR(10) NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    is_etf        BOOLEAN NOT NULL DEFAULT FALSE,
    description   VARCHAR(500) NOT NULL DEFAULT '',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ALTERs separados del CREATE de arriba: la tabla ya existia en produccion
-- desde antes de que estas columnas existieran, CREATE TABLE IF NOT EXISTS
-- no la toca en un despliegue sobre una BD ya inicializada.
ALTER TABLE tracked_symbols ADD COLUMN IF NOT EXISTS is_etf BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tracked_symbols ADD COLUMN IF NOT EXISTS description VARCHAR(500) NOT NULL DEFAULT '';

-- last_volume/last_volume_ts denormalizan el volumen del D1 mas reciente de
-- cada simbolo directo en tracked_symbols -- para ordenar /symbols/search
-- por volumen (mas relevante que alfabetico para un screener) sin tocar el
-- hypertable candles en cada busqueda. Se actualiza en Save() solo con
-- velas D1, comparando contra last_volume_ts para no dejar que un
-- re-fetch nocturno mas viejo (ver incrementalMargin) pise el valor mas
-- reciente si llega en otro orden dentro del mismo batch.
ALTER TABLE tracked_symbols ADD COLUMN IF NOT EXISTS last_volume BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tracked_symbols ADD COLUMN IF NOT EXISTS last_volume_ts TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_tracked_symbols_volume ON tracked_symbols (last_volume DESC);

-- OHLC/VWAP en DOUBLE PRECISION, no NUMERIC(18,6) -- mismo tipo que Go ya
-- usa internamente (float64), y algunos penny stocks reportan barras
-- historicas pre-reverse-split con valores que exceden el rango de
-- NUMERIC(18,6) (confirmado en vivo: overflow real en XXII/GWAV). Guardar
-- el valor real tal cual que reportar el feed es preferible a recortarlo o
-- descartar la barra entera.
CREATE TABLE IF NOT EXISTS candles (
    symbol_id   INT NOT NULL REFERENCES tracked_symbols(symbol_id),
    timeframe   VARCHAR(6) NOT NULL,
    ts          TIMESTAMPTZ NOT NULL,
    open        DOUBLE PRECISION NOT NULL,
    high        DOUBLE PRECISION NOT NULL,
    low         DOUBLE PRECISION NOT NULL,
    close       DOUBLE PRECISION NOT NULL,
    volume      BIGINT NOT NULL DEFAULT 0,
    trade_count INT,
    vwap        DOUBLE PRECISION,
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

-- last_ts refleja hasta donde llega la data real por simbolo+temporalidad,
-- actualizado en cada Save() (incluye M1 en vivo) y en cada agregacion
-- H1/D1 de medianoche -- consulta barata sin escanear candles.
CREATE TABLE IF NOT EXISTS watermarks (
    symbol_id  INT NOT NULL REFERENCES tracked_symbols(symbol_id),
    timeframe  VARCHAR(6) NOT NULL,
    last_ts    TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (symbol_id, timeframe)
);

-- dividend_amount/dividend_frequency de /market-data/by-type, refrescados en
-- el barrido nocturno (ver catchup.RefreshDividends). 0 significa "el emisor
-- no reparte dividendo" (SPAC, warrant, ETN, ETF apalancado sin
-- distribucion) -- confirmado con muestra real, no una ausencia de datos.
CREATE TABLE IF NOT EXISTS dividends (
    symbol_id          INT NOT NULL PRIMARY KEY REFERENCES tracked_symbols(symbol_id),
    dividend_amount    DOUBLE PRECISION NOT NULL DEFAULT 0,
    dividend_frequency DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
