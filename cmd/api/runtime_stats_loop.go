package main

import (
	"context"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
)

const runtimeStatsInterval = 3 * time.Minute

// StartRuntimeStatsLoop registra goroutines y memoria del proceso a
// intervalos regulares -- sin esto no hay forma de saber, mirando solo los
// logs, si la memoria crece de forma continua o a saltos ligados a un evento
// puntual (un resweep, un reinicio). El contenedor viene chocando contra su
// limite de memoria de forma recurrente desde julio (bumps de 256m a 2g sin
// resolver la causa de fondo), asi que esta traza queda permanente, no es
// un diagnostico de una sola vez -- no hay pprof expuesto todavia.
func StartRuntimeStatsLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(runtimeStatsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logRuntimeStats("periodic")
			}
		}
	}()
}

func logRuntimeStats(reason string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Info().
		Str("reason", reason).
		Int("goroutines", runtime.NumGoroutine()).
		Uint64("heapAllocMB", m.HeapAlloc/1024/1024).
		Uint64("heapSysMB", m.HeapSys/1024/1024).
		Uint64("sysMB", m.Sys/1024/1024).
		Uint32("numGC", m.NumGC).
		Msg("runtime stats")
}
