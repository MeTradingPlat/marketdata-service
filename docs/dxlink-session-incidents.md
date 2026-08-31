# Incidentes de "sessions exceeded" en DxLink -- bitácora

Antes de tocar de nuevo la reconexión/sesiones de DxLink, leer esto completo.
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
