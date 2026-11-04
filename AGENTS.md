# AGENTS.md

## Memoria

- 2026-11-04: Se perdieron señales por DxLinkConns huérfanas; arreglado en 9f5627b cerrando todas las conexiones al cambiar de fase.
- 2026-11-04: OOM (límite 2 GiB) en el rollout M1: StreamLive sin watermark repite ~8000 barras M1 por símbolo en un buffer sin límite y un único Save gigante. Arreglado en d0638a6: Save en trozos de 5000, flush anticipado con 20k pendientes, rollout pausado con más de 100k pendientes.
- 2026-11-04: El buffer de reintentos tenía tope 5000 y descartaba velas en silencio; ahora 100k. No bajarlo sin backpressure.
- 2026-11-04: Flush en vivo cada 60 s es deliberado (con 2 s: ~30 fsync/min en el disco saturado de VAIO, checkpoints de 200–300 s). La lectura no depende del flush: GetCandles combina recentCache.
- 2026-11-04: El barrido D1 iba a ~25 símbolos/min con Postgres al 385% de CPU: Save hacía UPDATE tracked_symbols last_volume una vez por vela D1 (unas 6750 veces la misma fila por transacción). Ahora es una vez por símbolo. El cuello de botella es Postgres, no SWEEP_WORKERS (60 workers ya rompió max_locks).
- 2026-11-04: candles tiene chunks de 1 día; D1 desde el año 2000 crea unos 6750 chunks.
- 2026-11-04: waitForData devuelve nil al vencer el timeout, así que un fetch vacío no deja error ni log (pendiente de revisar).
- 2026-11-04: Cada reinicio repite el M1 completo porque los guardados en vivo no ponen watermark (pendiente: arrancar desde la última vela M1 guardada).
- 2026-11-04: Servidor: ssh vaio. Usuario/BD con docker exec postgres-marketdata printenv POSTGRES_USER/POSTGRES_DB. La tabla candles tiene las columnas symbol_id/timeframe/ts (no symbol/time).
- 2026-11-04: El CD falla en lunes–viernes de 04:00 a 20:00 ET; fuera de esa ventana despliega solo.
- 2026-11-04: La compression policy de candles (job 1001) bloquea los INSERT del backfill D1 en chunks viejos (locks transactionid/tuple); el ciclo la pausa antes del sweep D1 y la reanuda tras M1.
- 2026-11-04: pgx + plan generico (tras 5 ejecuciones) sobre ~6750 chunks infla backends a ~1GB que no se liberan; plan_cache_mode=force_custom_plan en el CD y MaxConnIdleTime 5m.
- 2026-11-04: memory.high 3.2GiB en postgres causaba IO thrash (load 30) en backfill; subido a 3.8GiB (max 4GiB).
