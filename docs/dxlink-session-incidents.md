# Incidentes de conexiones/pool/memoria en marketdata-service -- bitácora

Cubre dos problemas relacionados pero distintos: el storm de "sessions
exceeded" de DxLink, y los OOM del contenedor ("se satura, solo arregla el
reinicio"). Antes de tocar de nuevo la reconexión/sesiones/memoria, leer
esto completo.
Varios intentos ya se probaron y fallaron o quedaron incompletos -- repetirlos
sin saberlo hace perder tiempo.

## El problema de fondo

TastyTrade impide autenticar una conexión DxLink nueva si el usuario ya tiene
"demasiadas" sesiones abiertas (`"The number of user sessions has exceeded
the configured limit"`). El límite es opaco (no documentado). Las sesiones
quedan huérfanas server-side cuando un contenedor muere sin cerrar limpio, o
cuando una tanda de reconexiones se dispara junta. Confirmado en vivo:

- **2026-08-17**: ~20 min sin feed.
- **2026-08-18 17:16Z**: se repite tras 5 reinicios de contenedor en 1h.
- **2026-08-18**: se descubre que `DELETE /sessions` (logout) devuelve
  **403 "Token has insufficient scopes"** -- el grant `refresh_token` solo
  trae scopes `read trade`. TastyTrade migró a OAuth-only (SDK v7, feb-2026)
  y quitó la gestión de sesiones vía ese endpoint. **El logout REST nunca
  funcionó y no se puede arreglar del lado del cliente.**
- **2026-08-25**: logout 403 + refill cayendo de 250k a 3k velas/hora.
- **2026-08-28**: silencio total de DxLink por horas en pleno mercado
  abierto, sin recuperarse solo -- varias conexiones mandaban su propio
  `DELETE /sessions` casi al mismo tiempo y cada una invalidaba la sesión
  recién creada por la anterior.
- **2026-08-29**: una tanda de reconexiones forzadas dejó el barrido D1
  parado ~2h -- causa distinta (ver más abajo), coincidió con un storm real.
- **2026-08-30/31**: storm sostenido 30+ min, 500-1000 errores/min sin bajar,
  mientras el barrido seguía pidiendo conexiones nuevas cada pocos segundos.
- **2026-10-15**: storm sostenido 44+ horas desde 2026-10-13 19:18Z sin
  resolverse solo (a diferencia de los incidentes de agosto). Causa: el
  SessionBreaker liberaba a TODOS los que esperaban en el mismo instante en vez
  de serializar el primer intento, re-saturando el límite de inmediato.

## Fixes aplicados, en orden (no repetir los descartados)

| Fecha | Commit | Qué se intentó | Resultado |
|---|---|---|---|
| pre-08-18 | `f6bcd6f` | Logout + refresh de token en `OnSessionReset` | **Descartado**: el logout siempre da 403 (scopes). El storm se resolvía solo por el TTL server-side, no por este fix -- quedó como *peso muerto* que se sigue llamando (barato, no hace daño) pero no arregla nada por sí solo. |
| 08-18/25 | -- | `connectionStaggerDelay = 200ms` antes de abrir cada conexión nueva del pool (`channel_allocator.go`) | Sigue vigente. Evita el caso de "rollout rápido pide muchas conexiones de golpe", pero NO evita el storm cuando el límite ya está saturado por sesiones viejas. |
| 08-28 | `7102626` | `singleflight.Group` en `OAuth.ResetSessions` (colapsa resets concurrentes en uno solo) | Sigue vigente. Necesario pero no suficiente -- soluciona que N conexiones se pisen el reset entre sí, no que N conexiones sigan reintentando. |
| 08-28 | `7102626` | `ResetLiveConnections()` en shutdown (close graceful) + reconciler de 5 min (`live_reconcile.go`) | Sigue vigente. Reduce sesiones huérfanas nuevas; no ataca un storm ya en curso. |
| -- | -- | `sessionSaturatedDelay = 15min` en `dxlink_reconnect.go`: cuando una conexión YA AUTENTICADA recibe `ERROR:UNAUTHORIZED` con "sessions" en el mensaje, espera 15 min antes de reintentar (en vez del backoff normal de segundos) | Sigue vigente, pero **solo cubre conexiones que llegaron a autenticarse alguna vez**. Ver el gap de abajo. |
| 2026-08-29 | `dd3197f` | `connDone` channel para desbloquear `dxLinkChannel.open()` cuando la conexión muere a mitad del handshake (deadlock de goroutines, no el storm en sí) | Bug relacionado pero distinto: sin esto, un storm de reconexión podía dejar workers del barrido colgados para siempre (sin error en el log, CPU en reposo) en vez de solo reintentar. |
| **2026-08-31** | **`0109c18`** | **`SessionBreaker` compartido por todo el proceso** (`session_breaker.go`) | **Fix actual, ver abajo.** |
| **2026-10-15** | -- | **`SessionBreaker.Guard`: gate serializado tras cooldown activo** (`session_breaker.go`, `dxlink_conn.go`) | **Fix actual, ver abajo.** |
| **2026-11-24** | -- | **`onReconnect` asíncrono con timeout propio + huérfanos reenganchables + reciclado de canales mudos** (`dxlink_reconnect_perform.go`, `candle_reconnect.go`, `live_orphan_heal.go`, `live_channel_heal.go`) | **Fix actual, ver abajo.** |

## El gap que quedaba sin cubrir (fix del 2026-08-31)

`sessionSaturatedDelay` solo se activaba cuando una conexión **ya
autenticada** recibía el error de protocolo `ERROR:UNAUTHORIZED` con
"sessions" en el texto. Pero las conexiones **nuevas** que el barrido
nocturno abre bajo demanda (`backfillWithRetry` -> `channelAllocator.allocate`
-> `connFactory`) reciben el rechazo de forma distinta cuando el límite ya
está saturado: TastyTrade cierra el WebSocket con un **close 1000 normal,
texto "Bye"**, antes de que el handshake AUTH termine -- nunca llega un
`ERROR:UNAUTHORIZED`, así que la bandera `sessionSaturated` de esa conexión
nunca se activaba. El llamador (`backfillWithRetry`) solo esperaba 5s y
reintentaba, con decenas de workers del barrido en paralelo -- eso es lo que
mantenía el storm sin bajar: cada intento nuevo competía por el mismo límite
saturado sin dejarle tiempo a las sesiones viejas de expirar.

**Fix**: `SessionBreaker` (nuevo, `internal/adapters/outgoing/external/tastytrade/session_breaker.go`) --
una ventana de 15 min (+ jitter de hasta 3s al vencer, para que no
reconecten todas de golpe) compartida por **TODAS** las conexiones del
proceso, no una bandera por conexión. Se marca desde los dos puntos de
rechazo (`ERROR:UNAUTHORIZED` en `dxlink_readloop.go` Y el close 1000
durante el handshake en `notifyHandshakeFailure`), y se consulta al inicio
de `DxLinkConn.Connect()` -- cubre tanto las reconexiones del pool en vivo
como las conexiones efímeras del barrido, porque ambas pasan por el mismo
`connFactory` (`di.go`).

Verificado en vivo: de 500-1000 errores/min a 0 tras el deploy, sweep H1
avanzando sin fallos de conexión.

## El gap de serialización tras cooldown (fix del 2026-10-15)

Confirmado en vivo el 2026-10-15: un storm sostenido de 44+ horas desde
2026-10-13 19:18Z no se resolvía solo. La causa: cuando la ventana de cooldown
compartida vencía, `SessionBreaker.Wait` liberaba a TODAS las conexiones en cola
en el mismo instante (salvo un jitter menor), provocando que disquen a la vez
y re-saturen de inmediato el límite antes de que la primera conexión termine de
establecerse.

**Fix**: `SessionBreaker.Guard` (`dxlink_conn.go`, `oauth.go`). Implementa un gate
(`sync.Mutex`) que serializa el primer intento de dial únicamente tras haber
atravesado una ventana de cooldown activa. Las demás conexiones en cola esperan
en el gate hasta que el dial en curso resuelve (éxito o re-saturación); si el
primer intento vuelve a saturar, el siguiente re-bloquea por la nueva ventana.
Cuando no hubo cooldown activo, `Guard` opera sin serialización añadida (no-op).

## El colapso por timeout de resubscripción (fix del 2026-11-24)

Confirmado en vivo el 2026-11-24: a las 17:02Z el pool colapsó (10.309
símbolos en `liveSubs` sin canal, 312 "dropped from live state" en 6 min,
racimos de hasta 96 símbolos congelados a la misma hora = canal entero mudo).
Causa raíz: `performReconnect` ejecutaba `onReconnect` con el MISMO
`reconnectStepTimeout` de 3 min que el handshake; con ~1.000 símbolos por
conexión la resubscripción se pasaba, el timeout llamaba `cleanup()` y cerraba
una sesión recién autenticada sana, y el bucle volvía a reconectar
(reconexión→timeout→reconexión). Además `handleConnectionReconnect` borraba
`liveSubs`/`liveTicks` al primer fallo de resubscripción, dejando al
reconciliador (100 símbolos/5 min) como único camino de recuperación.

**Fix**:
- `onReconnect` corre en su propio goroutine con `reconnectResubscribeTimeout`
  (10 min, `dxlink_reconnect.go`); `performReconnect` devuelve nil tras un
  "Connect" correcto y ya no cierra la sesión por el resultado de la
  resubscripción (solo loguea si la conexión se perdió mientras tanto).
- `pooledConnection.resubMu` serializa `handleConnectionReconnect` de la misma
  conexión (dos reconexiones pueden solaparse con la resubscripción asíncrona).
- Un fallo de resubscripción ya NO borra `liveSubs`/`liveTicks`: `liveSubs` pasa
  a ser la memoria de "este símbolo debe estar vivo" y el símbolo queda huérfano.
- `ReattachOrphans` (`live_orphan_heal.go`, 500/tick cada minuto, gracia 2 min):
  reengancha símbolos de `liveSubs` que no ocupan canal, con `FromTime` desde
  `lastClosed`.
- `HealMuteChannels` (`live_channel_heal.go`, 3/5 min, solo horario extendido y
  con el pool recibiendo eventos): recicla canales con ≥50 miembros, ≥90% mudos
  15 min y creados hace ≥30 min, vía `allocator.removeChannel` + `CHANNEL_CANCEL`;
  sus símbolos quedan huérfanos para `ReattachOrphans`.
- `StartLiveHealLoop` (`cmd/api/live_heal_loop.go`) corre ambos saneadores.

Verificación: `go build ./...` y `go vet` en verde; `go test -race` de
tastytrade, cmd/api, domain y catchup ok (servidor, docker golang:1.25).

## Incidente relacionado pero DISTINTO: OOM del contenedor ("se satura, solo arregla el reinicio")

No es lo mismo que el storm de sesiones de arriba -- ahí el proceso seguía
vivo y se recuperaba solo (o con el fix del breaker, ya no falla). Este otro
patrón es el **kernel matando el proceso por memoria** (`docker inspect`
mostraba `RestartCount=0`/sin auto-restart útil porque cada OOM exige que
alguien note el servicio caído y lo reinicie).

**Evidencia real** (`journalctl -k`, cgroup OOM, NO teoría):

| Fecha (UTC) | RSS al morir | Contexto |
|---|---|---|
| 2026-08-19 15:53 | -- | Durante el barrido normal D1/H1/M1 |
| 2026-08-19 16:16 | -- | 23 min después del anterior -- volvió a caer casi de inmediato |
| 2026-08-24 15:11 | ~1.02GB | Ya con el límite en 1g (ver abajo) -- lo tocó igual |

**Ya se "arregló" una vez y volvió a pasar**: el límite de memoria del
contenedor (`docker run --memory`, ver `cd.yml`) se subió de `512m` a `1g`
tras los dos OOM del 08-19 -- con el comentario de que el proceso llegó a
~518-522MB de RSS real. Pero el 08-24 volvió a pasar, esta vez tocando el
nuevo techo de 1GB casi exacto. **Conclusión: subir el límite solo pospone
el problema, no lo resuelve** -- el consumo real crece con la carga, no es
un tope fijo que ya se superó una vez.

Dos causas ya diagnosticadas (comentario de `cd.yml`):
1. El barrido D1/H1/M1 en sí (pool de hasta 40 conexiones + 25 workers).
2. `/marketdata/historical/batch`: arma la respuesta ENTERA (~9MB de JSON
   crudo por lote de 800 símbolos, ver `signal-processing-service`
   `_CANDLE_CHUNK_SIZE`) en memoria antes de comprimir con gzip. Varias
   llamadas grandes en paralelo (varios scanners evaluando a la vez) apilan
   varios buffers de ese tamaño al mismo tiempo.

**Fix del 2026-08-31 (`cfbc866`)**: semáforo (`MAX_CONCURRENT_BATCH_RESPONSES`,
default 4) que acota cuántas respuestas de `/historical/batch` se arman en
memoria a la vez -- ataca la causa #2 sin tocar el límite de memoria de
nuevo. La causa #1 (footprint del barrido en sí) sigue sin un fix dedicado
-- si el OOM vuelve a pasar específicamente durante la ventana del barrido
(00:05-01:00 UTC aprox.) y no durante uso normal de `/historical/batch`,
es la próxima señal a seguir.

**El host tiene memoria genuinamente escasa** (VAIO: 11GiB totales, ~3GiB
disponibles con todo lo demás corriendo -- ver [[reference_vaio_server]]):
subir el límite de este contenedor de nuevo compite en serio con los otros
~20 contenedores del mismo host, no es una perilla gratis.

**Antes de subir el límite de memoria otra vez**: confirmar primero que no
es la causa #1 sin resolver, y capturar `docker logs`/`docker stats` (o un
`pprof` si se agrega) ANTES de reiniciar -- cada reinicio borra el log del
contenedor anterior (se perdió toda evidencia de antes del 2026-08-31 por
esto mismo).

## Antes de tocar esto de nuevo

- El logout REST (`DELETE /sessions`) **no se puede arreglar** -- es un
  límite de scopes de TastyTrade, no un bug del cliente. No perder tiempo
  ahí de nuevo salvo que TastyTrade cambie los scopes del grant OAuth.
- Si vuelve un storm sostenido, lo primero es revisar si el rechazo llega
  como `ERROR:UNAUTHORIZED` (ya cubierto) o como algún OTRO patrón de cierre
  distinto a los dos ya manejados (`close 1000 "Bye"`) -- ver
  `dxlink_readloop.go`.
- Ver también [[reference_dxlink_session_limit]] y
  [[reference_java_dxlink_pool_sizing]] en la memoria de Claude para el
  contexto histórico previo a este archivo.

## Comentarios movidos desde el codigo (fase 6a)

### internal/adapters/outgoing/repository/timescale/candle_repository.go

**CandleRepository.writePool (campo)**

```go
// writePool es un pool chico DEDICADO a Save() -- las escrituras del
// streaming M1 nunca compiten por slots con las lecturas de charts (ver
// storage.writePoolConns): confirmado en vivo que 24 consultas de
// agregacion lentas ocupaban todas las conexiones del pool general y las
// escrituras de velas hacian fila detras con el mercado abierto.
```

**execWithDeadlockRetry (funcion)**

```go
// execWithDeadlockRetry reintenta ante un deadlock de Postgres (40P01) --
// mucho menos probable ahora que ya no hay una transaccion grande
// multi-simbolo compitiendo con Save() (ver daily_catchup.go), pero dos
// escrituras concurrentes para el MISMO simbolo (ej. catch-up diario y
// stream en vivo tocando el mismo D1 a la vez) todavia podrian chocar --
// reintentar es seguro porque todo es UPSERT.
```

**(CandleRepository).Save (metodo)**

```go
// Todo timeframe deja watermark aqui ahora -- antes solo M1 lo hacia (la
// vieja agregacion SQL H1/D1 ya no existe, asi que la fila ya no tiene con
// quien competir) y D1/H1 se apoyaban en MAX(ts) sobre candles directamente
// via GetWatermark. Eso resulto ser mucho mas caro de lo que parecia: sin
// un rango de fecha que acotar, Postgres no puede excluir chunks de la
// hypertable ni para PLANEAR la consulta (confirmado en vivo: hasta un
// EXPLAIN sin ejecutar tiraba "out of shared memory" con miles de chunks
// bloqueados de golpe). watermarks es una tabla chica sin particionar, una
// fila por simbolo+timeframe -- leerla es una busqueda por clave primaria,
// nunca toca la hypertable.
```

**maxWindowWidenAttempts/windowWidenFactor (const)**

```go
// maxWindowWidenAttempts/windowWidenFactor: ver el loop de ensanchamiento
// en GetCandles -- las zonas muertas grandes (fines de semana) dejaban la
// paginacion del frontend clavada en 0 barras. Bajado de 6 a 4 tras
// confirmar en vivo: cada intento extra de un simbolo ralo escaneaba
// chunks comprimidos mas viejos con decompresion vectorizada, y 10 queries
// asi en paralelo (30-90s cada una, ~7k locks) ahogaban el lock manager
// de postgres y todo el servicio se sentia lento.
```

**GetCandles (comentario previo a GetSeries)**

```go
// GetCandles arranca leyendo el watermark (la tabla chica, ver comentario
// de Save()) para calcular un piso de fecha real antes de tocar la
// hypertable -- sin eso, "dame las ultimas N barras" no le daba a Postgres
// ningun rango que excluir y sufria el mismo problema que GetWatermark
// antes de arreglarse: hasta planear la consulta bloqueaba chunks de mas.
// El margen base (2x bars + 30 periodos) cubre fines de semana/feriados/
// halts normales; zonas muertas mas largas se absorben ensanchando la
// ventana (ver el loop de abajo).
//
// before es nil para el chart inicial (las N barras mas recientes hasta el
// watermark) y no-nil para "cargar mas historial" al scrollear a la
// izquierda -- el frontend pide velas ANTES de la mas vieja que ya tiene
// (ver endDate en loadMoreHistory). Sin este parametro toda pagina
// devolvia siempre las mismas N barras mas recientes, asi que llegar al
// borde izquierdo nunca traia nada nuevo.
```

**(CandleRepository).GetSeries (metodo)**

```go
// GetSeries lee hasta bars velas por simbolo para un lote de simbolos en
// UNA query (row_number por particion de simbolo) -- el refresh de beta
// corria 13k queries individuales de 1300 barras D1 y saturaba la CPU de
// postgres ~40 min; con el mercado abierto las escrituras M1 en vivo hacian
// fila detras de la saturacion. Cada serie se devuelve ordenada por ts
// ascendente (mismo contenido que GetCandles por-simbolo).
```

**(CandleRepository).GetSeriesPriority (metodo)**

```go
// GetSeriesPriority es GetSeries por el pool chico de fundamentals/realtime
// (ver storage.snapshotPoolConns) -- confirmado en vivo el 2026-08-19: el
// fallback M1/D1 de GetSnapshotsBatch (para simbolos sin vela en vivo o sin
// subasta de cierre encontrada) llamaba GetSeries por el pool general, y con
// 3 escaneres reales pegandole al mismo tiempo esa consulta quedaba 23+
// segundos en espera de lectura de disco (DataFileRead) detras de las
// mismas agregaciones time_bucket que ya compiten por ese pool -- el mismo
// problema que motivo el pool dedicado, solo que esta llamada especifica
// nunca se movio para alla.
```

**seriesLookbackWindow (funcion)**

```go
// seriesLookbackWindow calcula cuanto retroceder en el tiempo para
// encontrar `bars` velas de una temporalidad dada -- funcion pura, separada
// de getSeriesFrom, para poder testearla sin tocar la base de datos.
//
// Sin cota de ts, ROW_NUMBER() OVER (PARTITION BY symbol_id ...) tiene que
// leer y ordenar TODA la historia de cada simbolo antes de recortar a
// `bars` -- para D1 eso son años de chunks comprimidos. Confirmado en vivo
// el 2026-08-21: una consulta de este tipo llevaba 68+ segundos activa
// bajo carga concurrente de escaneres (pg_stat_activity). JOIN LATERAL
// (la solucion que funciono para el mismo problema en seriesAggregatedBatchSQL)
// NO sirve aca: con D1 abarcando cientos de chunks el planner tarda 11+
// segundos SOLO planificando un lateral por simbolo. La cota de ts deja
// que TimescaleDB excluya chunks viejos enteros antes de siquiera
// descomprimirlos.
//
// minLookback pisa la formula cuando da una ventana absurdamente angosta
// -- SeedLastClose llama esto con bars=1 (M1: bars*3+60 = 63 MINUTOS), a
// proposito para sobrevivir el hueco entre el cierre de ayer y el primer
// tick de hoy (ver el comentario de SnapshotTracker.last). Confirmado en
// vivo el 2026-08-22: un arranque en fin de semana (horas despues del
// cierre del viernes) caia fuera de esos 63 minutos, la siembra volvia 0
// simbolos, y CADA lookup de precio en vivo terminaba cayendo al fallback
// individual de resolvePrice en vez de servirse del tracker -- 10 dias
// cubre de sobra cualquier fin de semana largo sin resucitar el problema
// original.
//
// D1+: ~5 dias habiles cada 7 calendario -- bars*3 sobreestima ~2x lo que
// realmente hace falta. Confirmado en vivo: pivots pide bars=1038 (4 anios
// de historial) y bars*3+60 daba una ventana de 8.7 anios, forzando
// escanear/descomprimir miles de chunks de mas para UN solo simbolo (37+s
// activo en pg_stat_activity, sin ningun lock de por medio). x1.6 (la
// proporcion real mas colchon de feriados) da ~4.7 anios para ese mismo
// caso -- margen comodo sin ser 2x mas ancho de lo necesario. Los
// timeframes intradia (M1/M5/H1/etc.) no tienen este problema -- su hueco
// de fin de semana es chico frente al total de `bars`, asi que siguen con
// la formula generica de siempre (igual de generosa que el ensanchamiento
// de ventana de GetCandles, sobra para los pedidos chicos que hace ese
// camino).
```

**GetCandles (ensanchamiento de la ventana)**

```go
// La ventana base (2x bars + 30 periodos) cubre feriados/halts normales,
// pero una zona muerta LARGA (un fin de semana entero, pausas de
// cotizacion en un simbolo iliquido) hace que la pagina caiga entera en
// cero velas -- y el frontend interpreta "pagina vacia" como "no hay mas
// historia" (hasMoreHistory=false, confirmado en vivo: el loadMore de
// MDXH M1 un domingo devolvia 0 barras con 951 velas del viernes justo
// detras). Por eso, si la pagina viene corta, se ensancha la ventana
// (x4) y se reintenta -- los simbolos densos pagan un solo query, los
// huecos cuestan un par de queries extra como mucho.
```

**GetCandles (slice vacio)**

```go
// candles arranca como slice vacio, no nil -- un query sin filas
// (ej. al scrollear mas alla del historial real) debe serializar
// como "[]" en la respuesta JSON, no como "null" (confirmado en
// vivo: un nil slice le rompia el `.length` al frontend, "Cannot
// read properties of null").
```

**GetCandles (historia real corta)**

```go
// Un simbolo con historia real corta (ETF apalancado recien listado,
// ej. MSTU con 481 D1 desde su lanzamiento) NUNCA junta `bars` sin
// importar cuanto se ensanche la ventana -- confirmado en vivo el
// 2026-08-20: sin este corte, las 4 vueltas de ensanche (hasta 256x
// la ventana base) igual repetian la misma consulta sobre un rango
// que ya cubria toda la historia real, costando 14s en vez de una
// sola consulta que ya tenia la respuesta correcta.
//
// "len(candles) == prevCount" SOLO corta si ya se encontro algo (> 0):
// contando 0 en dos vueltas seguidas NO prueba que no hay mas historia,
// solo que la ventana todavia no llego lo bastante atras -- un simbolo
// MUY iliquido (ej. CCRP, un ETF de bonos con literal 1 trade real en
// todo el dia) puede tener su unica vela real justo pasado el segundo
// ensanche y adentro del tercero. Confirmado en vivo el 2026-09-30:
// bars=10 (ventana base 50min, primer ensanche 200min) volvia "[]" para
// CCRP con antelacion de antes de que un attempt=2 (800min) SI hubiera
// encontrado la vela real a 251min de "anchor" -- se rendia despues de
// solo 1 ensanche sin haber agotado los 4 permitidos.
```

**(CandleRepository).GetPreviousSessionClose (metodo, to=16:01)**

```go
// to = 16:01 ET (no 16:02): la barra ts=16:00 es la subasta de cierre
// regular, la ts=16:01 ya es after-hours -- con la ventana [15:58,16:02)
// el ORDER BY ts DESC tomaba la vela post-cierre como prevClose cuando
// el simbolo opera despues del cierre (confirmado en vivo con MDXH:
// subasta 0.7998 vs 0.7548 de la barra 16:01).
```

**(CandleRepository).GetWatermarksBatch (metodo)**

```go
// GetWatermarksBatch lee los watermarks de un lote en una sola query -- el
// barrido por simbolo era 13k queries por fase, y bajo la carga de
// escrituras M1 cada una pagaba cola de IO (el refill media ~170s solo en
// lecturas de watermarks, confirmado en vivo el 2026-08-18).
```

**intradaySessionsSQL (const)**

```go
// intradaySessionsSQL agrega las M1 de hoy en las 3 sesiones (pre/regular/
// post) en una sola pasada. Los limites de tiempo se calculan en Go
// (dayStart/dayEnd/marketOpen/marketClose, ya en America/New_York) y se
// mandan como parametros -- comparar c.ts contra un valor ya calculado usa
// el indice (symbol_id, timeframe, ts) de verdad; la version anterior
// filtraba con (c.ts AT TIME ZONE ...)::date = hoy, una expresion por fila
// que Postgres no puede indexar y tardaba 30s en producción. ARRAY_AGG(...)[1]
// es el modismo para "primer/ultimo valor segun ese orden" sin una
// subconsulta correlacionada por sesion.
```

**universeBatchChunkSize (const)**

```go
// GetIntradaySessionsBatch es intradaySessionsSQL con GROUP BY s.symbol --
// ver el comentario del puerto (GetIntradaySessionsBatch) sobre por que esto
// reemplaza N queries secuenciales por una sola.
// universeBatchChunkSize: acota cuantos simbolos entran en un solo ANY($1)
// contra el universo activo entero (ver chunkSymbols) -- usado tanto por
// GetIntradaySessionsBatch (ARRAY_AGG por symbol, el caso mas caro: infló el
// snapshotPool a mas de 1GB de RSS por conexion el 2026-09-02, ver
// MaxConnLifetime en storage/timescale.go) como por
// GetPreviousSessionCloseBatch (DISTINCT ON, mucho mas liviano por fila,
// pero mismo principio de no mandarle los ~13k simbolos activos a Postgres
// de una sola vez). 9-10 lotes secuenciales acotan el pico de memoria por
// consulta sin perder ningun dato -- el resultado final es identico, solo
// mas repartido en el tiempo.
```

**previousSessionCloseWindowSQL (const)**

```go
// previousSessionCloseWindowSQL busca la subasta de UN dia especifico para
// el lote entero -- deliberadamente SIN el OR de prevSessionCloseDays
// ventanas que usa la version por simbolo. Confirmado en vivo el 2026-08-19:
// esa misma query con 10 ventanas OR-eadas y ~8800 simbolos en el ANY()
// tumbaba Postgres con "out of shared memory / max_locks_per_transaction"
// -- el planner necesita considerar los chunks candidatos de las 10 ramas a
// la vez dentro de una sola transaccion. Una ventana angosta por llamada
// solo toca 1-2 chunks (chunk_time_interval=7 dias), muy por debajo del
// limite.
```

### internal/adapters/outgoing/external/tastytrade/candle_live.go

**(CandlePool).RefreshLiveSubscriptions (metodo)**

```go
// RefreshLiveSubscriptions reenvia el Add M1 de cada simbolo ya suscrito,
// por su MISMO canal (nunca abre un canal o conexion nueva, ver
// channelAllocator.allChannels) -- un remove+add de la misma clave deja el
// streaming mudo para siempre (ver el comentario de subscribeLive), pero un
// Add repetido sobre una suscripcion que sigue viva es inofensivo: dxLink
// fusiona los "add" al mismo tema (ver el comentario de
// unsubscribeHistoryBatch) y, con FromTime distinto de cero, repite el
// historial desde ese punto antes de seguir en vivo por la misma
// suscripcion. Para una suscripcion que dejo de empujar datos en silencio
// (el caso que motivo esto, confirmado en vivo el 2026-09-04 con BSV/TW:
// canal y conexion sanos, cero eventos nuevos igual), este re-Add es la
// unica señal que le llega al servidor para reintentar sin que nosotros
// sepamos de antemano cual simbolo puntual esta mudo -- se manda a todos
// por igual, un mensaje por canal (no por simbolo), asi que el costo es
// proporcional a la cantidad de canales (~130-150), no al universo (~13k).
```

**refreshCycle (campo de CandlePool)**

```go
// refreshCycle alterna en dos mitades (canales pares/impares) cual grupo se
// refresca en cada llamada del ticker de 1 min -- confirmado en vivo el
// 2026-09-05: mandar los ~130-150 mensajes de TODOS los canales juntos, uno
// tras otro con solo 50ms de por medio, disparo un reenganche masivo de
// sesiones ("dispatch registration replaced" por miles en segundos) que
// termino saturando el limite de sesiones de TastyTrade minutos despues.
// Partiendo el barrido a la mitad, cada minuto manda la mitad de los
// mensajes (~65-75) y cada canal puntual se refresca cada 2 min en vez de
// cada 1 -- refreshLiveSubscriptionsFrom ya cubre exactamente esa ventana,
// asi que no queda ningun hueco sin repetir.
```

**(CandlePool).handleLiveEvent (metodo)**

```go
// handleLiveEvent detecta el cierre de una vela: mientras los eventos que
// llegan comparten el mismo timestamp, son actualizaciones de la vela en
// formacion; un timestamp MAS NUEVO significa que la anterior ya cerro. En
// los dos casos se reenvia la vela en formacion actualizada (dispatchTick)
// -- los graficos la necesitan tick a tick, no solo al cierre.
//
// Un timestamp MAS VIEJO que la vela en formacion (o igual/anterior a
// lastClosed cuando ya no hay "current" -- ver CloseElapsedForming) es un
// caso aparte: una correccion tardia de una vela YA cerrada (el trade real
// ocurrio en ese minuto pero el reporte del exchange/SIP llego despues de
// que ya cerramos el minuto siguiente -- documentado, no un caso raro: "el
// primer trade del minuto nuevo puede llegar varios segundos despues del
// cierre formal si el simbolo opera poco o hay demoras tecnicas"). Antes,
// el simple "timestamp distinto" de mas abajo confundia esto con una vela
// nueva -- cerraba de golpe la vela en formacion REAL con datos a medio
// completar y reabria la vieja como si fuera la actual. Confirmado en vivo
// el 2026-08-27/28: el volumen real de un pico de un minuto tardaba varios
// ciclos del escaner en reflejarse completo rio abajo. Ahora se despacha
// aparte via dispatchClosed, SIN tocar current -- fusionada contra
// lastClosed[symbol] (la ultima vela que SI cerro de verdad) para no perder
// los campos que dxLink no reenvio (formato COMPACT: solo manda lo que
// cambio). Si lastClosed no tiene ese timestamp exacto (la correccion apunta
// mas atras de una vela, caso raro), se fusiona sobre una base vacia como
// antes -- mejor esfuerzo, no hay de donde mas sacar el resto de los campos.
```

**handleLiveEvent (cierre organico)**

```go
// Cierre organico: llego un tick de un minuto mas nuevo. Sintetizar
// velas planas para los minutos intermedios sin tick (open=close al
// ultimo precio, volumen 0) parecia dar continuidad al grafico, pero
// el usuario lo rechazo en vivo el 2026-08-19: barras identicas donde
// no cambio nada ensucian la serie -- el chart debe mostrar barras
// solo donde hubo movimiento real (como antes de a19301a). Un minuto
// SIN ningun tick de por medio (silencio real) se cierra igual, pero
// por tiempo -- ver CloseElapsedForming.
```

**(CandlePool).flushFormingCandles (metodo)**

```go
// flushFormingCandles guarda la vela EN FORMACION de cada simbolo antes de
// que StopAllLive/CloseAllConnections la descarten -- confirmado en vivo el
// 2026-09-01: ninguno de los dos guardaba la vela a medio cerrar, asi que
// se perdia en silencio cada vez que el proceso se reinicia (hoy paso 6+
// veces) o en cada ventana de mantenimiento nocturna, hasta que el barrido
// M1 la volvia a traer de TastyTrade horas despues. Reusa dispatchClosed
// (mismo camino de guardado que un cierre real, con su chequeo IsComplete)
// en vez de inventar uno nuevo -- del lado de afuera es indistinguible de
// un cierre real, solo que forzado por el apagado en vez de un tick nuevo.
// Guardar por este camino usa withWatermark=false igual que un cierre
// normal, asi que si quedo incompleta el proximo barrido M1 la reemplaza
// con la version real de TastyTrade sin conflicto.
```

**(CandlePool).LiveSubscribed (metodo)**

```go
// LiveSubscribed dice si el stream M1 del simbolo esta ACTUALMENTE
// registrado en el pool -- el reconciliador (cmd/api) lo usa para
// detectar las muertes silenciosas (un resubscribe fallido tras una
// reconexion deja el stream mudo sin error visible, confirmado en vivo el
// 2026-08-18 con OSRH) y resuscribirlas aunque la marca del ingestor siga
// en true.
```

## Comentarios movidos desde el codigo (fase 6b)

### cmd/api/universe_cycle.go

**refreshWithRetry (funcion)**

```text
// refreshWithRetry reintenta un refresh idempotente del barrido nocturno --
// confirmado en vivo: postgres cayo en recovery mode justo durante el upsert
// de beta (SQLSTATE 57P03) y con un solo intento se perdieron beta, earnings
// y el refresh externo de SEC/FINRA hasta la ventana del dia siguiente. Un
// fallo transitorio de BD no deberia costar una noche entera de datos.
```

**runUniverseCycle (funcion)**

```text
	// Mismo motivo para el SnapshotTracker: sin este seed temprano, el
	// ranking de Search() por volumen de HOY (ver rankByTodayVolume) no
	// tiene nada del tracker hasta que termine el sweep M1 (hasta 20+ min),
	// y cae al last_volume de AYER para casi todo el universo -- confirmado
	// en vivo el 2026-09-03: SNXX con 116k de volumen real de hoy en
	// Postgres aparecia en el puesto #18 de la busqueda, detras de simbolos
	// con mucho menos volumen real, solo porque el tracker todavia no tenia
	// nada cargado. Se usa `tracked` (no `activeTracked`, que recien se
	// calcula despues de D1) -- el seed de mas abajo, tras el sweep M1,
	// reemplaza esto con el dato completo y ya filtrado.
```

**runUniverseCycle (funcion)**

```text
	// FASE 2: H1, desde cero sesiones (RunSweepPhase cierra al terminar) --
	// restaurada el 2026-09-11: H1 volvio a ser timeframe base con backfill
	// nativo propio (ver timeframe_aggregation.go), no un derivado de M1
	// calculado al vuelo.
```

**runUniverseCycle (funcion)**

```text
	// FASE 3: M1 en vivo (se queda suscrito) + prevClose (se calcula desde
	// las velas M1 de la sesion anterior, asi que va DESPUES del rollout
	// M1 -- corria antes con la tabla M1 vacia en un refill en frio y
	// calculaba 0). Sin verificacion de huecos de 10 dias: cada vela
	// guardada (en vivo y en refill) actualiza su watermark, asi que un
	// reinicio retoma desde el ultimo minuto guardado y el replay de la
	// suscripcion rellena solo el hueco de las horas caidas.
	// El sweep M1 es el REFILL que avanza el watermark M1.
```

### internal/core/service/catchup/daily_catchup.go

**marketCloseHourET (constante)**

```text
	// marketCloseHourET es el cierre del POST-MARKET extendido, 20:00 ET --
	// vuelto a este valor el 2026-08-20 a pedido expreso del usuario: beta y
	// otros calculos que dependen de la barra D1 de hoy necesitan el precio
	// de cierre real del dia (post-market incluido), no solo el de la subasta
	// de las 16:00 ET. El refill arranca postCloseDelay despues (00:05 UTC en
	// EDT, 01:05 UTC en EST). Nota historica: entre el 2026-08-18 y esta fecha
	// se uso el cierre REGULAR (16:00 ET) en su lugar, porque el post-market
	// atrasaba el refill 4 horas frente a lo esperado en ese momento (20:05
	// UTC) -- si esto vuelve a ser un problema, ese es el tradeoff a resolver,
	// no volver a un offset UTC fijo (ver el test de DST mas abajo).
```

**testSymbolSuffix (constante)**

```text
// testSymbolSuffix: la familia UTP/NASDAQ "Z..ZZT" (ZAZZT, ZCZZT, ZWZZT,
// ZXZZT...) -- confirmado en vivo el 2026-08-31: ZWZZT y ZXZZT (descripcion
// "NASDAQ TEST STOCK") se colaron al universo rastreado porque el filtro
// anterior solo enumeraba dos variantes puntuales (ZAZZT/ZCZZT) en vez del
// patron completo. Ninguna accion real termina en ZZT.
```

**reconcileUniverse (funcion)**

```text
	// activeSet se arma desde `real` (ya filtrado), no `active` -- `real :=
	// active[:0]` reutiliza el mismo array de abajo (filtro in-place comun en
	// Go), asi que `active` queda con su cola sin sobreescribir despues del
	// loop de arriba: iterarlo de nuevo aca colaba symbolos de prueba
	// (isTestSymbol) de vuelta al set de "activos", dejandolos is_active=true
	// para siempre porque Deactivate() nunca los veia como stale (confirmado
	// en vivo el 2026-08-31 con ZWZZT/ZXZZT).
```

**runPhase (funcion)**

```text
// runPhase corre UN timeframe hasta el final -- todos los workers drenan la
// cola y terminan (wg.Wait) antes de que el llamador pueda arrancar la
// fase siguiente. Barrera estricta a proposito: nada de H1 empieza mientras
// quede un solo D1 pendiente, ni al reves.
//
// Los simbolos sin datos todavia (uncovered) siguen por el backfill
// individual (probe de profundidad maxima), que son pocos. El resto se
// agrupa en lotes de sweepBatchSize: una sola suscripcion DxLink por lote
// con el FromTime de cada simbolo -- confirmado en vivo que la version
// anterior (una suscripcion por simbolo) hacia el barrido 100 veces mas
// lento en mensajes y se sentia si corria en horario de mercado.
```

**noWatermarkM1Fallback (constante)**

```text
// noWatermarkM1Fallback: cuanto pedir hacia atras para un simbolo M1 que
// "phaseJobs" clasifico como covered (SymbolsWithData ve al menos una fila)
// pero que en realidad NUNCA tuvo un backfill real -- solo velas del stream
// en vivo (withWatermark=false, ver ingest_candles.go), asi que no aparece
// en GetWatermarksBatch. Confirmado en vivo el 2026-08-19: sin este
// fallback, esos simbolos se caian en silencio del mapa `froms` (ni error
// ni log) y quedaban para siempre sin una sola vela M1 verificada -- miles
// de simbolos activos (AAPL, NVDA, AMD, GOOGL...) con semanas de datos sin
// verificar. 24h alcanza para lo que de verdad importa (hoy + el cierre de
// ayer) sin mandar a estos simbolos por el camino lento de a uno
// (backfillWithRetry/ProbeMaxDepth) que fue diseñado para simbolos NUEVOS
// sin ninguna fila todavia, no para miles de simbolos activos de golpe.
```

**saveBatchSweep (funcion)**

```text
// saveBatchSweep junta las velas cerradas de TODOS los simbolos del lote en
// un solo Save() -- antes era un Save por simbolo (hasta sweepBatchSize
// round-trips a Postgres para un lote que ya llego en UNA sola respuesta de
// DxLink). Save() ya arma un pgx.Batch heterogeneo por simbolo/timeframe
// (ver candle_repository.go), asi que mezclar simbolos en el mismo llamado
// es gratis. Si el guardado combinado falla, se cae al backfill individual
// por simbolo (mismo camino de siempre) en vez de perder el lote entero.
```

**saveBatchSweep (funcion)**

```text
	// Este es el camino PRINCIPAL del sweep M1 (el per-symbol de arriba solo
	// corre si el Save() en lote falla) -- sin este llamado, todo el catch-up
	// de arranque le seguia dejando al SnapshotTracker el volumen de ayer de
	// cada simbolo hasta el proximo reconcile contra la BD. Confirmado en
	// vivo el 2026-09-08: SMH (con su primera vela via stream en vivo)
	// rankeaba por encima de NVDA/TSLL/GPRO (con miles de acciones reales de
	// HOY ya en Postgres via este mismo sweep) simplemente porque el tracker
	// todavia no los habia visto.
```

**backfillWithRetry (funcion)**

```text
// backfillWithRetry reintenta un trabajo de backfill fallido -- confirmado en
// vivo: un apagon transitorio de Postgres (segundos) durante la fase D1/H1
// dejaba simbolos completos sin H1 hasta la siguiente noche, porque este
// camino nunca tuvo reintento (a diferencia de M1, ver startLiveWithRetry).
// A diferencia del callback de M1 en vivo -- compartido por cientos de
// simbolos en el mismo hilo de lectura del WebSocket, donde bloquear ahi
// afectaria a todos -- cada worker de este pool ya procesa su propia cola:
// reintentar aca solo demora a ESE worker, no a los demas.
```

**RunSweepPhase (funcion)**

```text
// RunSweepPhase ejecuta el barrido de UN timeframe (los pendientes + los
// cubiertos con margen) y cierra TODAS las conexiones DxLink al terminar --
// diseno original del usuario: cada fase termina, se desuscribe y se cierran
// las conexiones para asegurarse de que termino y de no arrastrar sesiones
// a la fase siguiente (el limite de sesiones de TastyTrade se superaba
// arrastrando las conexiones de una fase a la otra). El ciclo (ver
// universe_cycle.go) intercala los calculos de cada fase entre barridos:
// D1 -> beta/prevClose -> H1 -> M1.
```

## 2026-11-25: HealMuteChannels reciclaba canales sanos en pre-market

HealMuteChannels reciclaba canales sanos en pre-market: el silencio de un
canal con >=90% de miembros mudos es legitimo fuera de la sesion regular. La
reapertura de canales fallo porque dxLink estaba inestable y ReattachOrphans
reintentaba los huerfanos en serie (45 s por intento), dejando el pool
degradado.

Fix: HealMuteChannels solo actua en sesion regular 09:30-16:00 ET
(isRegularSessionET, session_regular.go) y ReattachOrphans corta el tick tras
3 fallos consecutivos (orphanReattachMaxConsecutiveFailures).

## 2026-11-25: Simbolos liquidos mudos tras barrido M1 (incidente NVDA)

Confirmado en vivo el 2026-11-25: tras el arranque (16:52Z), el barrido M1 de
`history-batch` cubrio NVDA (17:23:23Z), NVDX (17:21:57Z) y WULF (17:21:58Z).
El rollout live termino a las 17:34:25Z con 12.436 simbolos, pero a las 20:00Z
NVDA, NVDX y WULF seguian mudos desde las 17:20-17:22Z (~8 de cada 101
liquidos afectados, solo 5.341 con M1 reciente). El servidor dxLink deja de
entregar velas tras el Remove del batch y el Add posterior en otro canal, y el
refresh cada minuto por el mismo canal no lo repara.

Fix:
- Observabilidad (`candle_live_activity.go`, `live_health_log.go`):
  `lastRoutedAt`, `lastTradeAt`, `lastNewBarAt` (solo avanza con timestamp
  mayor al ultimo visto) y log `live health` cada minuto en `StartLiveHealLoop`.
- Proteccion de dispatch y batch (`candle_dispatch.go`, `candle_history.go`,
  `daily_catchup_backfill.go`): `dispatchEntry.source` impide que otro origen
  pise o borre un dispatch `live`; `FetchHistoryBatch` M1 excluye
  `LiveSubscribed` y `catchup` los cuenta aparte como `skipped_live`.
- `SubscribeLive` transaccional e idempotente (`candle_live.go`): revierte
  `liveTicks`, dispatch y ocupacion si falla el envio (conservando `liveSubs`).
- `HealSilentSymbols` (`live_silent_heal.go`, `live_silent_move.go`,
  `live_heal_loop.go`): en sesion regular y con el pool recibiendo eventos,
  repara el dispatch si falta o mueve el simbolo a un canal distinto
  (`allocateExcluding`) si lleva >5m sin barra nueva y tiene trade propio
  (DayVolume creciente <=5m), priorizando por volumen del dia descendente,
  con presupuesto 100/tick, corte tras 3 fallos y maximo 3 reparaciones/hora
  por simbolo.

## 2026-11-25: SESSION_RESET cerraba conexiones en vivo y pérdida de buffer al apagar

Causa y evidencia: `SESSION_RESET` (default 00:05 UTC = 19:05 ET en invierno,
dentro del post-market) llamaba a `ResetLiveConnections` (`CloseAllConnections`,
vaciando `liveSubs`) aunque `ResetSessions` solo renueva el token. A las 00:05Z
cerró 13 conexiones; a las 00:07Z el reconciler saltó el tick por `'no dxlink
connection available'` y después recuperó solo 100 símbolos cada 5 min hasta el
rollout nocturno (02:11Z, 12.437 símbolos). Además, al apagar el proceso no se
vaciaban los buffers de guardado en memoria.

Fix:
- `StartSessionResetLoop` (`cmd/api/session_reset_loop.go`) solo rota la sesión
  y ya no cierra conexiones DxLink.
- `CandlePool.CanSubscribeLive` (`live_availability.go`: alguna conexión
  conectada o pool vacío) sustituye a `Connected()` en `StartLiveReconcileLoop`
  (`cmd/api/live_reconcile.go`) y `ReattachOrphans` (`live_orphan_heal.go`).
- `flushSavesOnShutdown` (`cmd/api/shutdown_flush.go`, llamado en `cmd/api/main.go`
  con `shutdownFlushTimeout` = 8 s) vacía `liveSaveBuffer` y `retryBuffer` al
  apagar.

## 2026-11-26: miles de símbolos sin canal sin recuperación propia

### Evidencia (UTC, logs de producción)

| Hecho | Dato |
| --- | --- |
| Conexión cerrada y no reabierta | 12:51:46 a 14:05:07 (73 min) |
| Inicio del atasco de aperturas | 12:49 |
| Primera apertura abandonada por contexto vencido | 12:54:36 (356 en la ventana) |
| Timeouts de apertura de 45 s | 93, desde 12:56:07 |
| Símbolos sin canal | 7.137 constantes |
| Duración del ciclo de reparación | 405 s (9 aperturas de 45 s en cola) |

La recuperación llegó por el vigilante de silencio global. Con mercado abierto ese
vigilante no se dispara, porque las demás conexiones siguen recibiendo datos.

### Causas confirmadas en el código

- `Connect` pasaba el contexto del llamador a `readLoop`, `keepaliveLoop` y
  `healthCheckLoop`: al vencer, la conexión quedaba sin vigilancia.
- `OpenChannel` enviaba `CHANNEL_REQUEST` con el contexto vencido y, al fallar,
  no enviaba `CHANNEL_CANCEL`.
- La resuscripción tras reconectar seguía iterando con su plazo vencido.
- Dos reconexiones solapadas permitían asignar símbolos a canales del socket
  anterior.
- `ReviveSilentLive` corría en la goroutine del vigilante y no cortaba tras
  fallos.
- La cola de aperturas (`growMu`) no se podía cancelar y el asignador elegía
  siempre la primera conexión con hueco.

Sin probar: qué inició el atasco a las 12:49, anterior al primer abandono.

### Fix

- Ciclo de vida: los canales se marcan retirados al caer su conexión
  (`takeRetiredSymbols` separa lo que resuscribe cada manejador; desaparece
  `resubMu`). En `Connect` el contexto del llamador solo gobierna la espera de
  cooldown, el dial y el handshake; los bucles usan `context.WithoutCancel`,
  arrancan una vez y paran con `Close()`.
- Retomada (`live_resume.go`): cada símbolo conserva el punto más antiguo
  pendiente hasta que llega un dato pasados 2 minutos del primero. El tiempo sin
  datos no lo descarta.
- `UNAUTHORIZED` e `INVALID_MESSAGE` retiran los canales al momento.
- `Close()` cancela la espera de cooldown y la resuscripción en curso; la época
  del pool (`liveEpoch`) impide registrar símbolos después de cerrar.
- Asignador: espera cancelable (`growSem`); tras 3 timeouts de apertura seguidos,
  cuarentena de 2, 4, 8, 16 y 30 minutos más cierre y reconexión, como máximo una
  forzada por minuto. Con el pool en el tope, o si falla abrir una conexión
  nueva, se usa la conexión apartada con menos fallos que tenga hueco.
- Saneadores: `StartReviveSilentLive` no se solapa, tiene plazo de 3 minutos y
  corta tras 3 fallos; cada llamada de `runLiveHealTick` tiene plazo de 3 minutos.
  Durante `StopAllLive` y `CloseAllConnections` no suscriben.
- Calendario NYSE: sin sesión en festivos; en cierre anticipado la sesión regular
  acaba a las 13:00 ET y la extendida a las 17:00 ET. El vigilante ignora el
  silencio hasta las 04:03 ET. No cubre cierres extraordinarios.
- Trazas: `connID` en apertura, cierre, reconexión fallida, cuarentena y
  `BAD_ACTION`, que pasa a aviso.

### Diagnóstico: cómo identificar la causa

| Qué buscar | Qué dice |
| --- | --- |
| `dxlink socket closed`, campo `reason` | Quién cerró primero: error de lectura, silencio, fallo de envío, cuarentena, vigilante, cierre intencional, handshake o error del servidor |
| `dxlink channel open failed`, campo `probable_cause` | `network_silent`, `server_error`, `partial_response`, `server_slow` o `requests_ignored` |
| `dxlink late response on unregistered channel` | El servidor contesta después de rendirnos; `abandoned_age` dice cuánto |
| `dxlink server closed active channel` | El servidor corta un canal en uso; no se retira solo |
| Alerta `dxlink connection quarantined after channel open timeouts` en `log-service` | Foto de contadores; sobrevive a la recreación del contenedor |

`probable_cause` es una pista calculada, no una prueba. Los contadores son por
socket y se ponen a cero al reconectar.

### Comprobar tras desplegar

- A los 10 minutos de recuperar la red: 0 símbolos sin canal.
- Ninguna conexión con timeouts de apertura durante más de 5 minutos.
- Ningún `dxlink error` provocado por `CHANNEL_CANCEL`.
- Cada `dxlink session closed` no intencionado va seguido en 1 minuto de
  `opened` o de `dxlink reconnect failed`.
- Las goroutines no crecen con las reconexiones.
