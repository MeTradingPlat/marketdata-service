package configs

import (
	"strings"

	"github.com/spf13/viper"
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

func Load() *Config {
	viper.AutomaticEnv()

	viper.SetDefault("SERVER_PORT", "8082")
	viper.SetDefault("TEST_SYMBOLS", "AAPL")
	viper.SetDefault("TEST_MARKET", "XNAS")
	viper.SetDefault("TT_BASE_URL", "https://api.tastytrade.com")
	viper.SetDefault("MAX_CANDLE_POOL_CONNECTIONS", 40)
	viper.SetDefault("SWEEP_WORKERS", 25)
	// Reset diario de sesiones de TastyTrade a las 00:05 UTC (mercado US ya
	// cerrado desde las 00:00 UTC) -- ver StartSessionResetLoop.
	viper.SetDefault("SESSION_RESET_HOUR", 0)
	viper.SetDefault("SESSION_RESET_MINUTE", 5)
	// MAX_CONCURRENT_BATCH_RESPONSES: /marketdata/historical/batch arma la
	// respuesta ENTERA en memoria antes de comprimir (~9MB por lote de 800
	// simbolos, ver GzipWithConfig en router.go) -- confirmado por dmesg
	// (kernel OOM-killer) como una de las dos causas reales detras de los
	// OOM del 2026-08-19/24 (ver cd.yml). Sin un tope, N llamadas grandes en
	// paralelo (varios scanners evaluando a la vez) apilan N buffers de ese
	// tamano al mismo tiempo. 4 acota el peor caso sin frenar el uso normal.
	viper.SetDefault("MAX_CONCURRENT_BATCH_RESPONSES", 4)
	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", "5432")
	viper.SetDefault("DB_NAME", "marketdata_db")
	viper.SetDefault("DB_USERNAME", "user_marketdata")
	viper.SetDefault("DB_MAX_CONNS", 25)
	viper.SetDefault("EUREKA_HOST", "directory")
	viper.SetDefault("EUREKA_PORT", "8761")
	// El contenedor corre como usuario no-root "app" sin permiso de escritura
	// sobre /app (confirmado en vivo: MkdirAll fallaba con permission denied,
	// silenciosamente reduciendo el refresco de SEC EDGAR/insiders a un
	// no-op para las 13k+ simbolos del universo) -- /tmp si es escribible
	// para cualquier usuario (sticky bit 1777).
	viper.SetDefault("SEC_EDGAR_CACHE_DIR", "/tmp/secedgar-cache")

	return &Config{
		ServerPort:                  viper.GetString("SERVER_PORT"),
		TestSymbols:                 strings.Split(viper.GetString("TEST_SYMBOLS"), ","),
		TestMarket:                  viper.GetString("TEST_MARKET"),
		TastyTradeBaseURL:           viper.GetString("TT_BASE_URL"),
		TastyTradeClientID:          viper.GetString("TT_CLIENT_ID"),
		TastyTradeClientSecret:      viper.GetString("TT_CLIENT_SECRET"),
		TastyTradeRefreshToken:      viper.GetString("TT_REFRESH_TOKEN"),
		DxlinkURLOverride:           viper.GetString("DXLINK_URL"),
		MaxCandlePoolConnections:    viper.GetInt("MAX_CANDLE_POOL_CONNECTIONS"),
		SweepWorkers:                viper.GetInt("SWEEP_WORKERS"),
		SessionResetHour:            viper.GetInt("SESSION_RESET_HOUR"),
		SessionResetMinute:          viper.GetInt("SESSION_RESET_MINUTE"),
		MaxConcurrentBatchResponses: viper.GetInt("MAX_CONCURRENT_BATCH_RESPONSES"),
		DBHost:                      viper.GetString("DB_HOST"),
		DBPort:                      viper.GetString("DB_PORT"),
		DBName:                      viper.GetString("DB_NAME"),
		DBUser:                      viper.GetString("DB_USERNAME"),
		DBPassword:                  viper.GetString("DB_PASSWORD"),
		DBMaxConns:                  viper.GetInt("DB_MAX_CONNS"),
		EurekaHost:                  viper.GetString("EUREKA_HOST"),
		EurekaPort:                  viper.GetString("EUREKA_PORT"),
		SecEdgarCacheDir:            viper.GetString("SEC_EDGAR_CACHE_DIR"),
	}
}
