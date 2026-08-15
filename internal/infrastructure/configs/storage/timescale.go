package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

var (
	once     sync.Once
	instance *pgxpool.Pool
)

func ConnInstanceTimescale(cfg *configs.Config) *pgxpool.Pool {
	once.Do(func() {
		instance = connect(cfg)
	})
	return instance
}

// DSN en formato key=value, no URL -- una password con caracteres como
// #, &, * rompe el parseo de postgres://user:pass@host si se interpola
// cruda (confirmado en vivo: pgx la interpreta como fragmento/query de URL).
//
// MaxConns explicito -- sin esto pgxpool usa su default (max(4, NumCPU()),
// ~4 en el VAIO) que alcanza de sobra para trafico normal pero se satura
// con SweepWorkers concurrentes escribiendo a la vez: una lectura simple
// (ej. /marketdata/intraday) quedaba esperando hasta 15s por una conexion
// libre del pool, aunque la consulta en si tardara milisegundos.
func connect(cfg *configs.Config) *pgxpool.Pool {
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable pool_max_conns=%d",
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword, cfg.DBMaxConns)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to TimescaleDB")
	}
	log.Info().Msg("TimescaleDB connected successfully")
	return pool
}
