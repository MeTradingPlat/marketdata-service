# Arquitectura TastyTrade Integration - Market Data Service

## 🏗️ Diagrama de Arquitectura

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          MARKET DATA SERVICE                                 │
│                         (Spring Boot 3.5.9 + Java 21)                        │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                            KAFKA LISTENERS                                   │
│  ┌──────────────────────┐          ┌───────────────────────┐                │
│  │ orders.commands      │          │ marketdata.commands   │                │
│  │ (OrderRequest JSON)  │          │ (Subscribe/Unsubscribe)│                │
│  └──────────┬───────────┘          └───────────┬───────────┘                │
│             │                                   │                            │
│             ▼                                   ▼                            │
│  ┌──────────────────────┐          ┌───────────────────────┐                │
│  │ GestionarOrdersCU    │          │ GestionarRealTimeCU   │                │
│  │ Adapter              │          │ Adapter               │                │
│  └──────────┬───────────┘          └───────────┬───────────┘                │
└─────────────┼──────────────────────────────────┼───────────────────────────┘
              │                                   │
              │          ┌────────────────────────┘
              │          │
              ▼          ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                  GestionarComunicacionExternalGateway                        │
│                        (Output Port Interface)                               │
│  • sendOrder(OrderRequest)                                                   │
│  • subscribe(String symbol)                                                  │
│  • unsubscribe(String symbol)                                                │
│  • getCandles(symbol, timeframe, from, to)                                   │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         TASTYTRADE FACADE                                    │
│                    (Orchestration Layer)                                     │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ TastyTradeFacade                                                    │    │
│  │  • subscribe(symbol) → DxLink Quote + Trade                        │    │
│  │  • getCandles() → Check DB → Fetch from DxLink if gaps            │    │
│  │  • sendOrder() → REST API with dry-run                             │    │
│  └──────────────┬──────────────────────┬──────────────────────────────┘    │
└─────────────────┼──────────────────────┼───────────────────────────────────┘
                  │                      │
      ┌───────────┘                      └───────────┐
      │                                              │
      ▼                                              ▼
┌──────────────────────────────────┐   ┌──────────────────────────────────┐
│     DXLINK WEBSOCKET             │   │     REST API CLIENT              │
│   (Real-time Streaming)          │   │   (Orders + Authentication)      │
│                                  │   │                                  │
│ ┌──────────────────────────────┐ │   │ ┌──────────────────────────────┐ │
│ │ DxLinkWebSocketClient        │ │   │ │ TastyTradeAuthClient         │ │
│ │  - connect(url)              │ │   │ │  - getAccessToken()          │ │
│ │  - sendMessage(json)         │ │   │ │  - getApiQuoteToken()        │ │
│ │  - disconnect()              │ │   │ │    (OAuth 2.0 flow)          │ │
│ └──────────────────────────────┘ │   │ └──────────────────────────────┘ │
│                                  │   │                                  │
│ ┌──────────────────────────────┐ │   │ ┌──────────────────────────────┐ │
│ │ DxLinkConnectionManager      │ │   │ │ TastyTradeRestClient         │ │
│ │  States:                     │ │   │ │  - submitOrder(dto)          │ │
│ │  • DISCONNECTED              │ │   │ │  - dryRunOrder(dto)          │ │
│ │  • CONNECTING                │ │   │ │    POST /accounts/{id}/orders│ │
│ │  • AUTHENTICATED             │ │   │ └──────────────────────────────┘ │
│ │  • CHANNEL_READY             │ │   │                                  │
│ └──────────────────────────────┘ │   │ ┌──────────────────────────────┐ │
│                                  │   │ │ TokenRefreshScheduler        │ │
│ ┌──────────────────────────────┐ │   │ │  @Scheduled(23 hours)        │ │
│ │ DxLinkReconnectionStrategy   │ │   │ │  - refreshToken()            │ │
│ │  Exponential backoff:        │ │   │ │  - reconnect WebSocket       │ │
│ │  1s → 2s → 4s → ... → 60s    │ │   │ │  - restore subscriptions     │ │
│ └──────────────────────────────┘ │   │ └──────────────────────────────┘ │
│                                  │   └──────────────────────────────────┘
│ ┌──────────────────────────────┐ │
│ │ DxLinkKeepaliveScheduler     │ │
│ │  @Scheduled(30 seconds)      │ │
│ │  - sendKeepalive()           │ │
│ └──────────────────────────────┘ │
│                                  │
│ ┌──────────────────────────────┐ │
│ │ DxLinkMessageHandler         │ │
│ │  Parse JSON messages:        │ │
│ │  • SETUP                     │ │
│ │  • AUTH_STATE                │ │
│ │  • CHANNEL_OPENED            │ │
│ │  • FEED_DATA → dispatch      │ │
│ └──────────┬───────────────────┘ │
│            │                     │
│            ▼                     │
│ ┌──────────────────────────────┐ │
│ │ DxLinkChannelManager         │ │
│ │  - requestChannel(FEED)      │ │
│ │  - setupFeed(COMPACT)        │ │
│ │  - addSymbol(symbol, type)   │ │
│ │  - removeSymbol(symbol)      │ │
│ └──────────────────────────────┘ │
│                                  │
│ ┌──────────────────────────────┐ │
│ │ DxLinkSubscriptionRegistry   │ │
│ │  Thread-safe registry:       │ │
│ │  ConcurrentHashMap +         │ │
│ │  ReadWriteLock               │ │
│ └──────────────────────────────┘ │
│                                  │
│ ┌──────────────────────────────┐ │
│ │ EventHandlerRegistry         │ │
│ │  dispatch(eventType, data)   │ │
│ └──────────┬───────────────────┘ │
│            │                     │
│            ▼                     │
│ ┌──────────────────────────────┐ │
│ │ Event Handlers:              │ │
│ │  • QuoteEventHandler         │ │
│ │  • TradeEventHandler         │ │
│ │  • CandleEventHandler        │ │
│ └──────────┬───────────────────┘ │
└────────────┼─────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DATA OUTPUTS                                       │
│                                                                              │
│  ┌──────────────────────┐          ┌───────────────────────┐                │
│  │ KafkaProducerAdapter │          │ PostgreSQL            │                │
│  │  Topic:              │          │  (Historical Cache)   │                │
│  │  • marketdata.stream │          │                       │                │
│  │  • orders.updates    │          │ ┌───────────────────┐ │                │
│  │                      │          │ │ Candle Repository │ │                │
│  │  Publishes:          │          │ │  - saveCandles()  │ │                │
│  │  • Quote events      │          │ │  - countData()    │ │                │
│  │  • Trade events      │          │ │  - getHistorical()│ │                │
│  │  • Candle events     │          │ └───────────────────┘ │                │
│  │  • Order updates     │          │                       │                │
│  └──────────────────────┘          └───────────────────────┘                │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 🔄 Flujos de Datos Principales

### 1. Real-Time Subscription Flow

```
Kafka: marketdata.commands
  ↓
{"action": "SUBSCRIBE", "symbol": "AAPL"}
  ↓
GestionarRealTimeCUAdapter.subscribeToSymbol("AAPL")
  ↓
TastyTradeFacade.subscribe("AAPL")
  ↓
DxLinkSubscriptionRegistry.addSubscription("AAPL", null, QUOTE)
DxLinkSubscriptionRegistry.addSubscription("AAPL", null, TRADE)
  ↓
DxLinkChannelManager.addSymbol("AAPL", QUOTE)
DxLinkChannelManager.addSymbol("AAPL", TRADE)
  ↓
WebSocket: FEED_SUBSCRIPTION {add: [{"symbol": "AAPL", "type": "Quote"}]}
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
STREAMING PHASE (continuous)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
WebSocket: FEED_DATA {"data": ["Quote", ["Quote", "AAPL", 150.25, 150.27, ...]]}
  ↓
DxLinkMessageHandler.handleFeedData()
  ↓
EventHandlerRegistry.dispatch("Quote", data)
  ↓
QuoteEventHandler.handle(data)
  ↓
DxLinkEventMapper.toMarketDataStreamFromQuote(quote)
  ↓
KafkaProducerAdapter.publishMarketData(streamData)
  ↓
Kafka: marketdata.stream
  ↓
{"symbol": "AAPL", "bid": 150.25, "ask": 150.27, "lastPrice": 150.26, ...}
```

### 2. Historical Candles Flow

```
REST: GET /api/marketdata/historical/AAPL?timeframe=M5&from=...&to=...
  ↓
GestionarHistoricalDataCUAdapter.getHistoricalMarketData()
  ↓
CandleRepository.countData("AAPL", M5, from, to) → count = 50
calculateExpectedCandles(M5, from, to) → expected = 288
  ↓
IF count < expected:
  ↓
  TastyTradeFacade.getCandles("AAPL", M5, from, to)
    ↓
    candleSymbol = "AAPL{=5m}"
    fromTime = from.toInstant().toEpochMilli()
    ↓
    DxLinkChannelManager.addCandleSubscription(candleSymbol, fromTime)
    ↓
    WebSocket: FEED_SUBSCRIPTION {add: [{"symbol": "AAPL{=5m}", "type": "Candle", "fromTime": 1234567890}]}
    ↓
    ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    HISTORICAL DATA STREAMING
    ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    ↓
    WebSocket: FEED_DATA (múltiples mensajes con candles históricos)
    ↓
    CandleEventHandler.handle(data)
    ↓
    Parse → Candle domain model
    ↓
    CandleRepository.saveCandles([candle]) → PostgreSQL
    ↓
    IF subscriptionRegistry.hasActiveSubscription("AAPL"):
        KafkaProducerAdapter.publishMarketData(streamData)
    ↓
    CompletableFuture<List<Candle>>.complete()
    ↓
    DxLinkChannelManager.removeCandleSubscription(candleSymbol)
    ↓
ELSE:
  ↓
  CandleRepository.getHistoricalData("AAPL", M5, from, to) → List<Candle> (from cache)
  ↓
REST Response: List<Candle> (238 candles)
```

### 3. Order Submission Flow

```
Kafka: orders.commands
  ↓
{
  "symbol": "AAPL",
  "action": "BUY_TO_OPEN",
  "type": "LIMIT",
  "quantity": 10,
  "price": 150.00
}
  ↓
GestionarOrdersCUAdapter.placeBracketOrder(orderRequest)
  ↓
TastyTradeFacade.sendOrder(orderRequest)
  ↓
TastyTradeOrderMapper.toApiRequest(orderRequest)
  ↓
OrderRequestDTO {
  time-in-force: "Day",
  order-type: "Limit",
  price: 150.0,
  legs: [{instrument-type: "Equity", symbol: "AAPL", quantity: 10, action: "Buy to Open"}]
}
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
DRY-RUN PHASE (validation)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
TastyTradeRestClient.submitOrder(accountNumber, dto) // with dry-run
  ↓
POST /accounts/5WT00001/orders/dry-run
Headers: Authorization: Bearer {access_token}
Body: OrderRequestDTO
  ↓
Response: 200 OK {warnings: [], buying-power-effect: {...}}
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
ACTUAL ORDER SUBMISSION
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
POST /accounts/5WT00001/orders
Headers: Authorization: Bearer {access_token}
Body: OrderRequestDTO
  ↓
Response: 201 Created
OrderResponseDTO {
  id: "54758826",
  status: "Received",
  receivedAt: "2026-01-12T12:00:00Z"
}
  ↓
TastyTradeOrderMapper.toDomainResponse(apiResponse)
  ↓
OrderResponse {orderId: "54758826", status: "RECEIVED"}
  ↓
KafkaProducerAdapter.publishOrderUpdate(orderResponse)
  ↓
Kafka: orders.updates
  ↓
{
  "orderId": "54758826",
  "status": "RECEIVED",
  "receivedAt": "2026-01-12T12:00:00Z"
}
```

### 4. Token Refresh Flow (Every 23 hours)

```
@Scheduled(fixedRate = 23 hours)
  ↓
TokenRefreshScheduler.refreshToken()
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
STEP 1: Get new OAuth access token
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
TastyTradeAuthClient.getAccessToken()
  ↓
OAuthTokenRequestDTO {
  grant_type: "refresh_token",
  refresh_token: "{refresh_token}",
  client_id: "{client_id}",
  client_secret: "{client_secret}"
}
  ↓
POST /oauth/token
  ↓
OAuthTokenResponseDTO {
  access_token: "new_access_token_xxx",
  expires_in: 900 (15 minutes)
}
  ↓
TastyTradeProperties.setAccessToken(newAccessToken)
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
STEP 2: Get new API quote token
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
TastyTradeAuthClient.getApiQuoteToken()
  ↓
GET /api-quote-tokens
Headers: Authorization: Bearer {new_access_token}
  ↓
ApiQuoteTokenDTO {
  token: "new_api_quote_token_xxx",
  dxlink-url: "wss://tasty.dxfeed.com/realtime",
  level: "realtime"
}
  ↓
TastyTradeProperties.setApiQuoteToken(newApiQuoteToken)
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
STEP 3: Reconnect WebSocket
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
DxLinkConnectionManager.disconnect()
  ↓
WebSocket: CHANNEL_CANCEL {channel: 1}
  ↓
WebSocket: CLOSE
  ↓
Thread.sleep(1000)
  ↓
DxLinkConnectionManager.connect(newApiQuoteToken)
  ↓
WebSocket: CONNECT wss://tasty.dxfeed.com/realtime
  ↓
WebSocket: SETUP → AUTH → CHANNEL_REQUEST → FEED_SETUP
  ↓
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
STEP 4: Restore subscriptions
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ↓
DxLinkChannelManager.restoreSubscriptions()
  ↓
FOR EACH symbol in subscriptionRegistry.getAllSubscribedSymbols():
  ↓
  WebSocket: FEED_SUBSCRIPTION {add: [{"symbol": symbol, "type": "Quote"}]}
  ↓
✅ All subscriptions restored
  ↓
STREAMING RESUMES
```

---

## 🔐 Security & Thread-Safety

### Token Management
- **Access Token**: Renewed every 15 minutes (OAuth 2.0)
- **API Quote Token**: Renewed every 24 hours (for DxLink)
- **Storage**: volatile fields with synchronized getters/setters
- **Never logged**: Token values are masked in logs

### Thread-Safe Components
1. **DxLinkSubscriptionRegistry**: `ConcurrentHashMap` + `ReadWriteLock`
2. **TastyTradeProperties**: Synchronized accessors for volatile tokens
3. **WebSocket Session**: Synchronized with `sessionLock` in sendMessage()

### Connection Resilience
- **Exponential Backoff**: 1s → 2s → 4s → 8s → 16s → 32s → 60s (max)
- **Keepalive**: Every 30 seconds (timeout 60s)
- **Auto-reconnect**: On connection loss or AUTH_STATE UNAUTHORIZED
- **Subscription Restore**: After reconnection, all active subscriptions are restored

---

## 📊 Data Flow Characteristics

| Flow Type | Latency | Throughput | Storage |
|-----------|---------|------------|---------|
| **Real-Time Quote/Trade** | < 100ms | 100+ events/sec | Kafka only (ephemeral) |
| **Real-Time Candles** | < 1s | 10-20 events/sec | PostgreSQL + Kafka |
| **Historical Candles** | 5-10s | Batch (288 candles) | PostgreSQL (cached) |
| **Order Submission** | 200-500ms | 1-5 orders/sec | Kafka updates |

---

## 🛠️ Configuration Profiles

### Dev Profile (`application-dev.yml`)
- **PostgreSQL**: localhost:5432
- **Kafka**: localhost:9092
- **DxLink**: **tasty.dxfeed.com/realtime** (PRODUCTION WebSocket - real market data)
- **TastyTrade API**: **api.tastytrade.com** (PRODUCTION - live trading)
- **Logging**: DEBUG level
- **Purpose**: Local development con datos reales

### Prod Profile (`application-prod.yml`)
- **PostgreSQL**: AWS RDS (SSL required)
- **Kafka**: Production Kafka cluster
- **DxLink**: **tasty.dxfeed.com/realtime** (PRODUCTION WebSocket - live trading)
- **TastyTrade API**: **api.tastytrade.com** (PRODUCTION)
- **Logging**: INFO level
- **Purpose**: AWS deployment

**NOTA IMPORTANTE**: Ambos perfiles usan el entorno de **PRODUCCIÓN** de TastyTrade. No hay configuración de sandbox/demo por defecto.

---

## 📦 Technology Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| **Java** | OpenJDK | 21 |
| **Spring Boot** | Spring Boot | 3.5.9 |
| **Database** | PostgreSQL | (configurable) |
| **Message Broker** | Apache Kafka | (configurable) |
| **WebSocket Client** | Spring WebSocket | (included in Boot) |
| **REST Client** | Spring RestClient | (included in Boot) |
| **Object Mapping** | MapStruct | 1.6.3 |
| **Retry Logic** | Spring Retry | (Boot version) |
| **Environment Variables** | dotenv-java | 3.0.0 |
| **Testing** | JUnit 5 + Awaitility | (Boot version) |
| **Lombok** | Lombok | (Boot version) |

---

**Desarrollado con ❤️ para MetradingPlat**
