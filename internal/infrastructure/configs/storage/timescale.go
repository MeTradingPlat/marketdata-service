package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

var (
	once     sync.Once
	instance *pgxpool.Pool

	writeOnce     sync.Once
	writeInstance *pgxpool.Pool

	snapshotOnce     sync.Once
	snapshotInstance *pgxpool.Pool
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

// snapshotPoolConns: pool DEDICADO a fundamentals/realtime (intraday
// sessions + prevClose en lote) -- confirmado en vivo el 2026-08-19: con el
// pool general ocupado por las agregaciones time_bucket de H1/D1 de OTROS
// escaneres corriendo en paralelo (el mismo problema de recalculo en vivo
// que getAggregatedCandlesSQL ya documenta, pendiente de arreglar con un
// continuous aggregate), el batch ya optimizado de fundamentals/realtime
// igual se quedaba sin conexion libre y tardaba mas de 120s. 3 conexiones
// aisladas evitan que ese hueco tumbe al escaner de turno mientras se hace
// el arreglo de fondo.
const snapshotPoolConns = 3

func SnapshotPoolInstanceTimescale(cfg *configs.Config) *pgxpool.Pool {
	snapshotOnce.Do(func() {
		snapshotInstance = connectWithSize(cfg, snapshotPoolConns)
	})
	return snapshotInstance
}

func connect(cfg *configs.Config) *pgxpool.Pool {
	return connectWithSize(cfg, cfg.DBMaxConns)
}

// maxConnLifetime/maxConnLifetimeJitter: Postgres nunca le devuelve al SO la
// memoria que un backend allocated para una consulta pesada -- se queda con
// ese tamaño mientras la conexion siga viva, sin importar que despues quede
// idle. GetIntradaySessionsBatch/GetPreviousSessionCloseBatch (snapshotPool,
// solo 3 conexiones) agregan por symbol_id sobre los ~13k simbolos del
// universo entero de una sola pasada -- confirmado en vivo el 2026-09-02:
// 2 de esas 3 conexiones quedaron en ~1.2GB de RSS cada una tras
// seedSnapshotTracker (corre en CADA arranque del proceso, no solo una vez
// al dia) y nunca bajaron, contribuyendo a empujar al host entero a swap
// thrashing. Sin un limite explicito de vida por conexion, no hay nada que
// alguna vez le devuelva esa memoria a Postgres sin un reinicio manual. El
// jitter evita que las 3 conexiones del mismo pool expiren todas juntas.
const (
	maxConnLifetime       = 30 * time.Minute
	maxConnLifetimeJitter = 5 * time.Minute
)

func connectWithSize(cfg *configs.Config, maxConns int) *pgxpool.Pool {
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable pool_max_conns=%d",
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword, maxConns)

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse TimescaleDB DSN")
	}
	poolCfg.MaxConnLifetime = maxConnLifetime
	poolCfg.MaxConnLifetimeJitter = maxConnLifetimeJitter

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
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
