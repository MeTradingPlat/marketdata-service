# AGENTS.md

## Memoria

- 2026-11-04: Se perdieron señales por DxLinkConns huérfanas; arreglado en 9f5627b cerrando todas las conexiones al cambiar de fase.
- 2026-11-04: OOM (límite 2 GiB) en el rollout M1: StreamLive sin watermark repite ~8000 barras M1 por símbolo en un buffer sin límite y un único Save gigante. Arreglado en d0638a6: Save en trozos de 5000, flush anticipado con 20k pendientes, rollout pausado con más de 100k pendientes.
- 2026-11-04: El buffer de reintentos tenía tope 5000 y descartaba velas en silencio; ahora 100k. No bajarlo sin backpressure.
- 2026-11-04: Flush en vivo cada 60 s es deliberado (con 2 s: ~30 fsync/min en el disco saturado de VAIO, checkpoints de 200–300 s). La lectura no depende del flush: GetCandles combina recentCache.
- 2026-11-04: El barrido D1 iba a ~25 símbolos/min con Postgres al 385% de CPU: Save hacía UPDATE tracked_symbols last_volume una vez por vela D1 (unas 6750 veces la misma fila por transacción). Ahora es una vez por símbolo. El cuello de botella es Postgres, no SWEEP_WORKERS (60 workers ya rompió max_locks).
- 2026-11-04: candles tiene chunks de 1 día; D1 desde el año 2000 crea unos 6750 chunks.
- 2026-11-04: waitForData devuelve nil al vencer el timeout; ahora el barrido reporta simbolos vacios y lotes cortados (ErrPartialHistory) por fase, y un lote cortado se guarda sin avanzar watermark.
- 2026-11-04: Cada reinicio repite el M1 completo porque los guardados en vivo no ponen watermark (pendiente: arrancar desde la última vela M1 guardada).
- 2026-11-04: Servidor: ssh vaio (fuera de la LAN: ssh vaio-dns; su reloj es ET, el TZ local no es fiable). Usuario/BD con docker exec postgres-marketdata printenv POSTGRES_USER/POSTGRES_DB. La tabla candles tiene las columnas symbol_id/timeframe/ts (no symbol/time).
- 2026-11-04: El CD falla en lunes–viernes de 04:00 a 20:00 ET; fuera de esa ventana despliega solo.
- 2026-11-04: La compression policy de candles (job 1001) bloquea los INSERT del backfill D1 en chunks viejos (locks transactionid/tuple); el ciclo la pausa antes del sweep D1 y la reanuda tras M1.
- 2026-11-04: pgx + plan generico (tras 5 ejecuciones) sobre ~6750 chunks infla backends a ~1GB que no se liberan; plan_cache_mode=force_custom_plan en el CD y MaxConnIdleTime 5m.
- 2026-11-04: memory.high 3.2GiB en postgres causaba IO thrash (load 30) en backfill; subido a 3.8GiB (max 4GiB).
- 2026-11-04: D1/H1 viven en candles_history (chunks de 30 dias); M1 sigue en candles (1 dia). Enrutado por candleTable(tf) en el repo timescale.
- 2026-11-04: Los logs/alertas van a log-service por WebSocket (LOG_SERVICE_WS_URL, ws://log-service:8084/ws/internal/logs), no por Kafka; paquete alerts (Send/Notify) y binario cmd/notify que usa el CD al fallar.
- 2026-11-04: Los endpoints /debug exigen la cabecera X-Gateway-Passed si se llaman sin pasar por el gateway.
- 2026-11-04: fetchHistory omite M1 para simbolos con suscripcion en vivo; 184.195.238.144 es dxFeed (no es un ataque).
- 2026-11-04: oauth_test era flaky porque el servidor respondia antes de que llegaran todos los callers; el handler ahora espera a todos.
- 2026-11-04: Backups: ~/bin/pg-backup.sh en VAIO (cron 03:30), sube a rclone remote gdrive; restore verificado en las 3 BD chicas, marketdata_db pendiente.
- 2026-11-04: En local no hay gcc: go test -race solo corre en CI (Linux).
- 2026-11-12: Límites de la cuenta TastyTrade/DxLink: ~16 sesiones y ~10 canales por conexión (el canal 11 da timeout al abrir; inferido de los datos, tastytrade no lo ha confirmado). Con 8 canales/conn el rollout live usaba 17 sesiones y una conexión era rechazada y rotaba cada ~17 min; con 12 el canal 11 caía en un bucle de reconexión cada 3 min (661 símbolos sacados del live).
- 2026-11-12: Live M1 empaqueta LIVE_MAX_CHANNELS_PER_CONN=10 (~11 sesiones), los barridos siguen en 8 y el pool tiene MAX_CANDLE_POOL_CONNECTIONS=15 (commits ab82bb9 y 44cb7e07d). Verificado con 10: 0 timeouts de canal, 0 "exceeded the configured limit", 0 símbolos caídos. Si vuelven los timeouts, bajar a 9.
- 2026-11-12: Warrants, units y rights (~771 símbolos, domain.ClassifySecurityType) se excluyen solo del live y del reconciler (liveEligible en cmd/api); siguen en los barridos D1/H1/M1.
- 2026-11-12: Tras un cierre 1012 de dxFeed ~8% de los simbolos liquidos resuscritos quedan mudos (17/223 vs 0.4% base). Fix: lastEventAt por simbolo + pendingRevive + ReviveSilentLive en el watchdog (3 min mudo, max 2 intentos, 50/tick, sin unsubscribe). Causa raiz sin probar; verificar que reviven con los logs "revived silent live symbol".
- 2026-11-12: Un CI puede quedar en cola horas y el CD se bloquea después de las 04:00 ET; relanzar con `gh workflow run "CD Metradingplat MarketData Service" --ref master -f force=true`. El run 43095818925 quedó fantasma (queued, sin job; no se puede cancelar ni borrar: 409/403).
- 2026-11-13: Bug del techo del pool (~540 errores/hora "candle pool at connection ceiling (15)"): universe_cycle.go devolvía el límite a 8 canales/conexión tras el rollout live, y las conexiones reconectadas se reempaquetaban a 8 y desbordaban (124 canales necesarios vs 120). Fix f22b2ea: se mantiene LIVE_MAX_CHANNELS_PER_CONN. Desplegado; 1 h sin errores de techo, falta ver una reconexión real.
