# MarketData Service

Microservicio de datos de mercado para la plataforma **MeTradingPlat**. Provee candles
historicas, intraday, fundamentals, quotes en tiempo real y listado de simbolos/mercados,
integrando con la API REST de **TastyTrade** (OAuth 2.0) y el WebSocket **DxLink** de
dxFeed.

Implementado en **Go**, con arquitectura hexagonal (puertos y adaptadores).

## Arquitectura

```
cmd/api/                                  # entrypoint + loops de background (catchup, reconcile, etc.)
internal/
  core/
    domain/                               # entidades y calculos puros de negocio
    ports/
      in/                                 # interfaces de casos de uso
      out/                                # interfaces que el dominio necesita del exterior
    service/                              # implementacion de casos de uso, sin dependencias de framework
      catchup/                            # backfill/reconcile nocturno del universo de simbolos
      fundamentals/
      ingestion/                          # backfill, live-stream y buffering de candles
      intraday/
      livecandles/                        # broadcaster, cache M1 reciente, agregacion de barra actual
      metadata/                           # cache/busqueda de simbolos
  adapters/
    incoming/
      handler/                            # handlers HTTP (Echo) y WebSocket
    outgoing/
      external/
        tastytrade/                       # cliente DxLink WebSocket + cliente REST TastyTrade (OAuth)
        finra/                            # short interest
        secedgar/                         # shares/insiders SEC EDGAR
      repository/timescale/               # repositorios Postgres/TimescaleDB
  infrastructure/
    configs/
      injector/di.go                      # contenedor de DI (go.uber.org/dig)
      router/                             # registro de rutas Echo
      storage/                            # pool de conexion a la BD
    discovery/                            # cliente Eureka
    middleware/                           # logging, backfill gate, chequeo de header del gateway
```

Separacion consistente: los handlers en `adapters/incoming/handler` solo parsean/validan
y llaman a `ports/in`; la logica de negocio (cacheo de candles, agregacion por timeframe,
watermarks) vive en `internal/core/service/*`; el acceso a HTTP/DB/WebSocket queda
aislado en `internal/adapters/outgoing/*`.

## Tecnologias

| Tecnologia | Proposito |
| --- | --- |
| Go 1.25 | Lenguaje principal |
| Echo v4 | Router HTTP |
| gorilla/websocket | Conexiones WebSocket (DxLink + clientes) |
| pgx/v5 | Driver Postgres/TimescaleDB |
| go.uber.org/dig | Inyeccion de dependencias |
| viper | Configuracion (env vars, hot-reload) |
| zerolog | Logging estructurado |
| golang.org/x/sync (singleflight) | Colapsar resets de sesion OAuth concurrentes |
| Docker | Contenedorizacion multi-stage (`golang:1.25` -> `alpine:3.20`) |

## API Endpoints

Base path: `/marketdata` (sin prefijo `/api`). Todas las rutas reales estan en
`internal/infrastructure/configs/router/router.go`.

| Metodo | Path | Descripcion |
| --- | --- | --- |
| `GET` | `/marketdata/health` | Health check |
| `GET` | `/marketdata/historical/{symbol}` | Candles historicas (`timeframe`, `endDate`, `bars`) |
| `GET` | `/marketdata/candles/{symbol}/current` | Barra en formacion |
| `POST` | `/marketdata/historical/batch` | Candles historicas para multiples simbolos |
| `GET` | `/marketdata/intraday/{symbol}` | Snapshot intraday |
| `GET` | `/marketdata/fundamentals/{symbol}` | Fundamentals de un simbolo |
| `POST` | `/marketdata/fundamentals/realtime` | Fundamentals para una lista de simbolos |
| `POST` | `/marketdata/quotes/rest` | Precios actuales (nombre de ruta preservado por compatibilidad con `signal-processing-service`) |
| `GET` | `/marketdata/symbols` | Simbolos filtrados por mercado |
| `GET` | `/marketdata/symbols/search` | Busqueda de simbolos (`q`, `page`, `size`, `markets`) |
| `GET` | `/marketdata/symbols/{symbol}/details` | Detalle de un simbolo |
| `GET` | `/marketdata/markets` | Lista de mercados |
| `GET` | `/marketdata/timeframes` | Lista de timeframes soportados |
| `GET` | `/marketdata/debug/probe-depth/{symbol}` | Diagnostico puntual (no llamar seguido) |
| `GET` | `/ws/candles` | WebSocket de candles en vivo (frame `{action, symbol, timeframe}`) |
| `GET` | `/ws/snapshot` | WebSocket de snapshots intraday |
| `GET` | `/ws/fundamentals` | WebSocket de fundamentals |

`GET /marketdata/historical/batch` responde gzip (nivel `BestSpeed`) cuando el cliente
manda `Accept-Encoding: gzip` — el payload de un batch grande puede llegar a varios MB de
JSON. Durante un backfill/refill, `middleware.BackfillGate` deja pasar siempre las
lecturas livianas (fundamentals, symbols, quotes) y limita en concurrencia solo las
pesadas (historical/intraday/candles).

## Integraciones externas

### TastyTrade REST (OAuth 2.0)

El servicio renueva el access token automaticamente y rota el refresh token en memoria
(`internal/adapters/outgoing/external/tastytrade/oauth.go`) — no se persiste en disco,
por lo que un reinicio del contenedor vuelve a partir del `TT_REFRESH_TOKEN` de entorno.
Los resets de sesion concurrentes se colapsan con `singleflight` para evitar rechazos de
autenticacion en paralelo.

### DxLink WebSocket (dxFeed)

Pool de conexiones multiplexadas (`internal/adapters/outgoing/external/tastytrade/`):
hasta `MAX_CANDLE_POOL_CONNECTIONS` conexiones (default 40), cada una con varios canales,
cada canal con varios simbolos suscritos. Los canales de streaming en vivo se
reconectan automaticamente; los canales de fetch historico son efimeros y se cierran al
completar la solicitud o al expirar el timeout.

## Configuracion (variables de entorno)

| Variable | Default | Descripcion |
| --- | --- | --- |
| `SERVER_PORT` | `8082` | Puerto HTTP |
| `TT_BASE_URL` | `https://api.tastytrade.com` | Base URL de TastyTrade |
| `TT_CLIENT_ID` | — | Client ID OAuth (requerido) |
| `TT_CLIENT_SECRET` | — | Client Secret OAuth (requerido) |
| `TT_REFRESH_TOKEN` | — | Refresh token inicial (requerido, rota en runtime) |
| `DXLINK_URL` | — | Override de la URL del WebSocket DxLink |
| `MAX_CANDLE_POOL_CONNECTIONS` | `40` | Conexiones DxLink para el pool de candles |
| `SWEEP_WORKERS` | `25` | Workers para el ciclo de rollout D1->H1->M1 |
| `SESSION_RESET_HOUR` / `SESSION_RESET_MINUTE` | `0` / `5` | Hora UTC del reset diario de sesion TastyTrade |
| `MAX_CONCURRENT_BATCH_RESPONSES` | `4` | Tope de respuestas de `/historical/batch` en paralelo (protege memoria) |
| `DB_HOST` / `DB_PORT` / `DB_NAME` / `DB_USERNAME` / `DB_PASSWORD` | `localhost` / `5432` / `marketdata_db` / `user_marketdata` / — | Conexion Postgres/TimescaleDB |
| `DB_MAX_CONNS` | `25` | Tamano del pool de conexiones |
| `EUREKA_HOST` / `EUREKA_PORT` | `directory` / `8761` | Registro de servicio |
| `SEC_EDGAR_CACHE_DIR` | `/tmp/secedgar-cache` | Cache de datos SEC EDGAR (debe ser escribible por el usuario del contenedor) |

## Ejecucion

### Con Docker Compose (recomendado)

Desde la raiz del proyecto `metradingplat/`:

```bash
docker compose up -d
docker compose logs -f marketdata-service
```

El servicio esta disponible en `http://localhost:8082` (directo) o
`http://localhost:8080/marketdata` (via Gateway).

### Desarrollo local

```bash
cd marketdata-service
go run ./cmd/api
```

Requiere Go 1.25+, Postgres/TimescaleDB y Eureka corriendo (o las variables de entorno
correspondientes apuntando a instancias existentes).

### Tests

```bash
go vet ./...
go test ./...
```

## Limitaciones conocidas

- **Refresh token OAuth**: se rota en memoria en cada renovacion; un reinicio del proceso
  despues de una rotacion cae de nuevo al valor de `TT_REFRESH_TOKEN` de entorno, que
  puede estar desactualizado.
- **Symbol sin validar en el borde**: los endpoints REST/WS no validan formato/longitud
  del parametro `symbol` — bajo riesgo porque las queries a la BD estan parametrizadas,
  pero permite strings arbitrarios en cache keys y llamadas a TastyTrade/DxLink.
- **`/marketdata/historical/batch`**: arma la respuesta completa en memoria antes de
  comprimir; `MAX_CONCURRENT_BATCH_RESPONSES` acota cuantas llamadas grandes corren en
  paralelo para evitar OOM (incidente real, ver `cd.yml`/comentarios en `config.go`).
