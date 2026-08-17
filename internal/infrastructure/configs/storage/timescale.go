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

	writeOnce     sync.Once
	writeInstance *pgxpool.Pool
)

func ConnInstanceTimescale(cfg *configs.Config) *pgxpool.Pool {
	once.Do(func() {
		instance = connect(cfg)
	})
	return instance
}

// writePoolConns: pool DEDICADO a escrituras (velas del streaming M1) --
// 4 conexiones que nunca compiten con las lecturas de charts por slots:
// confirmado en vivo que 24 consultas de agregacion lentas ocupaban las 25
// conexiones del pool general y las escrituras de velas hacian fila detras
// (watermark M1 clavado 10+ minutos con el mercado abierto). NO se subio el
// limite general (pool_max_conns=80 ya rompio max_locks_per_transaction en
// el pasado, ver comentario de connect): 4 conexiones de escritura extra
// son inocuas para locks y memoria.
const writePoolConns = 4

func WritePoolInstanceTimescale(cfg *configs.Config) *pgxpool.Pool {
	writeOnce.Do(func() {
		writeInstance = connectWithSize(cfg, writePoolConns)
	})
	return writeInstance
}

func connect(cfg *configs.Config) *pgxpool.Pool {
	return connectWithSize(cfg, cfg.DBMaxConns)
}

func connectWithSize(cfg *configs.Config, maxConns int) *pgxpool.Pool {
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable pool_max_conns=%d",
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword, maxConns)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to TimescaleDB")
	}
	log.Info().Msg("TimescaleDB connected successfully")
	return pool
}

// DSN en formato key=value, no URL -- una password con caracteres como
// #, &, * rompe el parseo de postgres://user:pass@host si se interpola
// cruda (confirmado en vivo: pgx la interpreta como fragmento/query de URL).
//
// pool_max_conns=20, no 80 -- se probo en 80 para aliviar la cola de
// conexiones bajo SweepWorkers=60, pero eso empujo el total de locks
// simultaneos (conexiones x chunks de la hypertable, ~1390 hoy) mas alla de
// max_locks_per_transaction en Postgres, tumbando el barrido entero con
// "out of shared memory" y saturando el VAIO. 20 sigue siendo mas que el
// default implicito (~4, max(4, NumCPU)) sin acercarse al techo que rompio
// todo -- ajustar de nuevo solo junto con max_locks_per_transaction del
// lado de Postgres, no por separado.
