package configs

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	ServerPort  string
	TestSymbols []string
	TestMarket  string

	TastyTradeBaseURL        string
	TastyTradeClientID       string
	TastyTradeClientSecret   string
	TastyTradeRefreshToken   string
	DxlinkURLOverride        string
	MaxCandlePoolConnections int
	SweepWorkers             int

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
}

func Load() *Config {
	viper.AutomaticEnv()

	viper.SetDefault("SERVER_PORT", "8082")
	viper.SetDefault("TEST_SYMBOLS", "AAPL")
	viper.SetDefault("TEST_MARKET", "XNAS")
	viper.SetDefault("TT_BASE_URL", "https://api.tastytrade.com")
	viper.SetDefault("MAX_CANDLE_POOL_CONNECTIONS", 40)
	viper.SetDefault("SWEEP_WORKERS", 80)
	viper.SetDefault("DB_HOST", "localhost")
	viper.SetDefault("DB_PORT", "5432")
	viper.SetDefault("DB_NAME", "marketdata_db")
	viper.SetDefault("DB_USERNAME", "user_marketdata")

	return &Config{
		ServerPort:               viper.GetString("SERVER_PORT"),
		TestSymbols:              strings.Split(viper.GetString("TEST_SYMBOLS"), ","),
		TestMarket:               viper.GetString("TEST_MARKET"),
		TastyTradeBaseURL:        viper.GetString("TT_BASE_URL"),
		TastyTradeClientID:       viper.GetString("TT_CLIENT_ID"),
		TastyTradeClientSecret:   viper.GetString("TT_CLIENT_SECRET"),
		TastyTradeRefreshToken:   viper.GetString("TT_REFRESH_TOKEN"),
		DxlinkURLOverride:        viper.GetString("DXLINK_URL"),
		MaxCandlePoolConnections: viper.GetInt("MAX_CANDLE_POOL_CONNECTIONS"),
		SweepWorkers:             viper.GetInt("SWEEP_WORKERS"),
		DBHost:                   viper.GetString("DB_HOST"),
		DBPort:                   viper.GetString("DB_PORT"),
		DBName:                   viper.GetString("DB_NAME"),
		DBUser:                   viper.GetString("DB_USERNAME"),
		DBPassword:               viper.GetString("DB_PASSWORD"),
	}
}
