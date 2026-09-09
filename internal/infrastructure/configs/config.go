package configs

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ServerPort  string
	TestSymbols []string
	TestMarket  string

	TastyTradeBaseURL           string
	TastyTradeClientID          string
	TastyTradeClientSecret      string
	TastyTradeRefreshToken      string
	DxlinkURLOverride           string
	MaxCandlePoolConnections    int
	SweepWorkers                int
	SessionResetHour            int
	SessionResetMinute          int
	MaxConcurrentBatchResponses int

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBMaxConns int

	EurekaHost string
	EurekaPort string

	SecEdgarCacheDir string
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func Load() *Config {
	return &Config{
		ServerPort:             envOr("SERVER_PORT", "8082"),
		TestSymbols:            strings.Split(envOr("TEST_SYMBOLS", "AAPL"), ","),
		TestMarket:             envOr("TEST_MARKET", "XNAS"),
		TastyTradeBaseURL:      envOr("TT_BASE_URL", "https://api.tastytrade.com"),
		TastyTradeClientID:     os.Getenv("TT_CLIENT_ID"),
		TastyTradeClientSecret: os.Getenv("TT_CLIENT_SECRET"),
		TastyTradeRefreshToken: os.Getenv("TT_REFRESH_TOKEN"),
		DxlinkURLOverride:      os.Getenv("DXLINK_URL"),

		MaxCandlePoolConnections: envIntOr("MAX_CANDLE_POOL_CONNECTIONS", 40),
		SweepWorkers:             envIntOr("SWEEP_WORKERS", 25),
		// Reset diario de sesiones de TastyTrade a las 00:05 UTC (mercado US ya
		// cerrado desde las 00:00 UTC) -- ver StartSessionResetLoop.
		SessionResetHour:   envIntOr("SESSION_RESET_HOUR", 0),
		SessionResetMinute: envIntOr("SESSION_RESET_MINUTE", 5),
		// MAX_CONCURRENT_BATCH_RESPONSES: /marketdata/historical/batch arma la
		// respuesta ENTERA en memoria antes de comprimir (~9MB por lote de 800
		// simbolos, ver GzipWithConfig en router.go) -- confirmado por dmesg
		// (kernel OOM-killer) como una de las dos causas reales detras de los
		// OOM del 2026-08-19/24 (ver cd.yml). Sin un tope, N llamadas grandes en
		// paralelo (varios scanners evaluando a la vez) apilan N buffers de ese
		// tamano al mismo tiempo. 4 acota el peor caso sin frenar el uso normal.
		MaxConcurrentBatchResponses: envIntOr("MAX_CONCURRENT_BATCH_RESPONSES", 4),

		DBHost:     envOr("DB_HOST", "localhost"),
		DBPort:     envOr("DB_PORT", "5432"),
		DBName:     envOr("DB_NAME", "marketdata_db"),
		DBUser:     envOr("DB_USERNAME", "user_marketdata"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBMaxConns: envIntOr("DB_MAX_CONNS", 25),

		EurekaHost: envOr("EUREKA_HOST", "directory"),
		EurekaPort: envOr("EUREKA_PORT", "8761"),

		// El contenedor corre como usuario no-root "app" sin permiso de escritura
		// sobre /app (confirmado en vivo: MkdirAll fallaba con permission denied,
		// silenciosamente reduciendo el refresco de SEC EDGAR/insiders a un
		// no-op para las 13k+ simbolos del universo) -- /tmp si es escribible
		// para cualquier usuario (sticky bit 1777).
		SecEdgarCacheDir: envOr("SEC_EDGAR_CACHE_DIR", "/tmp/secedgar-cache"),
	}
}
