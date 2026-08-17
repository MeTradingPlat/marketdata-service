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

-- Resto de fundamentales de TastyTrade (/market-data/by-type +
-- /market-metrics, ver tastytrade/market_metrics.go), agregadas a la misma
-- tabla en vez de crear una nueva -- mismo symbol_id, mismo ciclo de vida.
-- Columnas separadas por endpoint de origen para que cada refresco solo
-- toque las suyas via UPSERT (ver FundamentalsRepository).
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS trading_status TEXT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS halt_start_time BIGINT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS halt_end_time BIGINT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS market_data_updated_at TIMESTAMPTZ;

ALTER TABLE dividends ADD COLUMN IF NOT EXISTS market_cap DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS eps DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS beta DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS lendability TEXT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS borrow_rate DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS liquidity DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS liquidity_rating INT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS implied_volatility_index DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS implied_volatility_rank DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS implied_volatility_percentile DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS next_earnings_date TEXT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS metrics_updated_at TIMESTAMPTZ;

-- shares_outstanding/float_shares/short_interest/short_ratio no vienen de
-- TastyTrade (confirmado contra /market-metrics, /market-data/by-type e
-- /instruments/equities reales, ninguna los trae) -- vienen de SEC EDGAR
-- (companyfacts.zip + insider/institutional ownership bulk data) y FINRA
-- (CSV quincenal de interes en corto). external_updated_at es NULL hasta
-- que ese refresco corre por primera vez para el simbolo -- distinto de
-- "corrio y no encontro dato" (que deja la columna en NULL tambien, pero
-- external_updated_at si queda seteado -- la app distingue por esa fecha,
-- no por si el valor es NULL).
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS shares_outstanding BIGINT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS float_shares BIGINT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS short_interest DOUBLE PRECISION;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS short_ratio DOUBLE PRECISION;
-- short_interest_shares es el dato CRUDO de FINRA (acciones en corto); el
-- porcentaje se calcula al momento de leer contra el float vigente, para
-- que nunca quede desincronizado cuando el float cambia despues (ver
-- domain.ShortInterestPercent). short_interest_settlement permite
-- descartar releases de FINRA demasiado viejas (publicacion quincenal,
-- con mas de ~5 semanas de atraso no es dato util).
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS short_interest_shares BIGINT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS short_interest_settlement TEXT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS external_updated_at TIMESTAMPTZ;
-- insider_shares/insider_ciks son intermedios de RefreshExternalFundamentals
-- (Form 3/4/5 bulk) que RefreshBeneficialOwners lee despues para calcular
-- floatShares real sin restar dos veces la posicion de un insider que
-- tambien sea holder 5%+. float_updated_at solo se toca cuando floatShares
-- pasa de heuristico (90% de sharesOutstanding) a real, para poder rotar
-- por "el que hace mas tiempo no se refresca de verdad" (ver
-- GetSymbolsDueForFloatRefresh).
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS insider_shares BIGINT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS insider_ciks TEXT[];
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS float_updated_at TIMESTAMPTZ;
-- occurred_date es la fecha del ultimo reporte de earnings REAL (con EPS
-- ya publicado) -- viene de un endpoint separado de TastyTrade
-- (historic-corporate-events/earnings-reports), por-simbolo, sin batch.
-- earnings_updated_at marca cuando corrio ese refresco por ultima vez.
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS occurred_date TEXT;
ALTER TABLE dividends ADD COLUMN IF NOT EXISTS earnings_updated_at TIMESTAMPTZ;

-- Log de cuando se calculo cada refresh diario de fundamentales (beta,
-- earnings, market metrics, externos) -- un paso se salta en reinicios
-- dentro de la misma ventana de mantenimiento (cierre de mercado +
-- postCloseDelay, ver daily_catchup.NextMaintenanceWindowAt): el done_at
-- persiste en postgres, asi un redeploy a mitad de dia no recalcula lo que
-- ya se calculo tras el cierre. Solo se registra un paso que termino OK.
CREATE TABLE IF NOT EXISTS fundamental_refresh_log (
    step TEXT PRIMARY KEY,
    done_at TIMESTAMPTZ NOT NULL
);
