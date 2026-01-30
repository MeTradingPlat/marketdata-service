# MarketData Service

Microservicio de datos de mercado con integracion a TastyTrade (OAuth 2.0, DxLink WebSocket, REST API para ordenes).

## Arquitectura

```mermaid
graph TB
    subgraph Input["Input Adapters"]
        KC[Kafka Consumers<br/>orders.commands<br/>marketdata.commands]
        REST[REST Controllers<br/>Historical, Markets, Quote<br/>Earnings, Orders]
    end

    subgraph UseCases["Use Cases"]
        UCOrders[GestionarOrdersCU]
        UCRealTime[GestionarRealTimeCU]
        UCHistorical[GestionarHistoricalDataCU]
        UCMercados[GestionarMercadosCU]
        UCQuote[GestionarQuoteCU]
        UCEarnings[GestionarEarningsCU]
    end

    subgraph Output["Output Adapters"]
        Gateway[GestionarComunicacionExternalGateway]
        KP[Kafka Producer<br/>marketdata.stream<br/>orders.updates]
        DB[(PostgreSQL<br/>Historical Cache)]
    end

    subgraph TastyTrade["TastyTrade Integration"]
        Facade[TastyTradeFacade]
        Auth[TastyTradeAuthClient<br/>OAuth 2.0]
        RestClient[TastyTradeRestClient<br/>Orders + Instruments]
        DxLink[DxLink WebSocket<br/>Real-time streaming]
    end

    KC --> UCOrders & UCRealTime
    REST --> UCHistorical & UCMercados & UCQuote & UCEarnings

    UCOrders & UCRealTime & UCHistorical --> Gateway
    UCMercados & UCQuote & UCEarnings --> Gateway
    Gateway --> Facade

    Facade --> Auth & RestClient & DxLink
    DxLink --> KP & DB
    UCOrders --> KP
```

## Flujo Real-Time

```mermaid
sequenceDiagram
    participant K as Kafka (commands)
    participant UC as GestionarRealTimeCU
    participant F as TastyTradeFacade
    participant WS as DxLink WebSocket
    participant KP as Kafka (stream)

    K->>UC: SUBSCRIBE AAPL
    UC->>F: subscribe("AAPL")
    F->>WS: FEED_SUBSCRIPTION Quote + Trade
    loop Streaming
        WS-->>F: FEED_DATA (Quote/Trade)
        F-->>KP: marketdata.stream
    end
```

## Flujo Historical Candles

```mermaid
sequenceDiagram
    participant C as Client
    participant UC as GestionarHistoricalDataCU
    participant DB as PostgreSQL
    participant F as TastyTradeFacade
    participant WS as DxLink WebSocket

    C->>UC: GET /historical/AAPL?timeframe=M5
    UC->>DB: countData(AAPL, M5, from, to)
    alt Cache incompleto
        UC->>F: getCandles(AAPL, M5, from, to)
        F->>WS: FEED_SUBSCRIPTION Candle
        WS-->>F: FEED_DATA (candles)
        F->>DB: saveCandles()
    end
    DB-->>UC: List<Candle>
    UC-->>C: JSON response
```

## Flujo Bracket Order (OTOCO)

```mermaid
sequenceDiagram
    participant C as Client
    participant UC as GestionarOrdersCU
    participant F as TastyTradeFacade
    participant TT as TastyTrade REST

    C->>UC: POST /orders (bracket)
    UC->>F: sendBracketOrder()
    F->>TT: POST /accounts/{id}/complex-orders
    Note over TT: OTOCO: Entry + StopLoss + TakeProfit
    TT-->>F: 201 Created
    F-->>UC: OrderResponse
    UC-->>C: {orderId, status}
```

## REST API

| Metodo | Endpoint | Descripcion |
|--------|----------|-------------|
| GET | `/api/marketdata/historical/{symbol}` | Candles historicos (`?timeframe=M5&from=...&to=...`) |
| GET | `/api/marketdata/markets` | Mercados disponibles (NYSE, NASDAQ, AMEX, ETF, OTC) |
| GET | `/api/marketdata/symbols` | Simbolos por mercado (`?markets=NYSE,NASDAQ`) |
| GET | `/api/marketdata/quote/{symbol}` | Quote actual (bid, ask, last, halt status) |
| GET | `/api/marketdata/earnings/{symbol}` | Proximo earnings report |
| POST | `/api/marketdata/orders` | Enviar orden bracket (OTOCO) |
| DELETE | `/api/marketdata/orders/{orderId}` | Cancelar orden |

## Kafka Topics

| Topic | Direccion | Payload |
|-------|-----------|---------|
| `orders.commands` | IN | `{symbol, action, quantity, type, price, stopLossPrice, takeProfitPrice}` |
| `marketdata.commands` | IN | `{symbol, action: SUBSCRIBE/UNSUBSCRIBE}` |
| `orders.updates` | OUT | `{orderId, status, receivedAt}` |
| `marketdata.stream` | OUT | `{symbol, lastPrice, bid, ask, volume, timestamp}` |

## Stack

| Componente | Tecnologia |
|------------|------------|
| Runtime | Java 21 + Spring Boot 3.5.9 |
| Base de datos | PostgreSQL |
| Mensajeria | Apache Kafka |
| WebSocket | DxLink (TastyTrade) |
| Mapping | MapStruct 1.6.3 |
| Auth | OAuth 2.0 (refresh_token) |
| Service Discovery | Eureka |

## Configuracion

### Variables de entorno requeridas

```bash
TT_CLIENT_ID=...
TT_CLIENT_SECRET=...
TT_REFRESH_TOKEN=...
TASTYTRADE_ACCOUNT_NUMBER=...
DXLINK_URL=wss://tasty.dxfeed.com/realtime  # produccion
```

### Perfiles

- **dev**: PostgreSQL localhost:5432, Kafka localhost:9092, Eureka localhost:8761
- **prod**: PostgreSQL Docker, Kafka Docker, Eureka Docker

### Compilar y ejecutar

```bash
mvnw.cmd clean compile -DskipTests
mvnw.cmd spring-boot:run
```

El servicio inicia en puerto 8082 y se registra en Eureka. Solo acepta peticiones a traves del Gateway (puerto 8080, header `X-Gateway-Passed`).

## Arquitectura Hexagonal

```mermaid
graph LR
    subgraph Input["Input (Driving)"]
        R[REST Controllers]
        K[Kafka Listeners]
    end

    subgraph Application["Application"]
        IP[Input Ports<br/>CUIntPort]
    end

    subgraph Domain["Domain"]
        UC[Use Cases<br/>CUAdapter]
        M[Models]
    end

    subgraph AppOut["Application"]
        OP[Output Ports<br/>GatewayIntPort]
    end

    subgraph Output["Output (Driven)"]
        GW[Gateway Adapter]
        KP[Kafka Producer]
        DB[(PostgreSQL)]
    end

    R & K --> IP --> UC
    UC --> M
    UC --> OP --> GW & KP & DB
```

## Seguridad

- **Tokens**: Access token (15 min) + API quote token (24h), renovacion automatica
- **Thread-safety**: ConcurrentHashMap + ReadWriteLock en suscripciones, volatile + synchronized en tokens
- **Reconexion**: Backoff exponencial 1s -> 60s max, keepalive cada 30s, restore de suscripciones post-reconexion
- **Gateway filter**: Solo acepta requests con header `X-Gateway-Passed: true`
