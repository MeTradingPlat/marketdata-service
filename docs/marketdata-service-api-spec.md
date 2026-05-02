# 📘 Especificación Técnica: Integración con MarketData Service

Este documento define el contrato de API y los mecanismos de sincronización para que otros microservicios (Orders, UI, Quantitative) consuman datos del ecosistema TastyTrade/dxLink.

## 📡 Arquitectura de Entrega
El servicio opera bajo un modelo **Híbrido de Alta Performance**:
1.  **REST (Pull)**: Para estructuras pesadas o datos históricos.
2.  **Streaming (Push/Cache)**: Los datos volátiles se suscriben vía WebSocket (dxLink) al primer request.

---

## 🛠 Endpoints REST Disponibles

### 1. Market Data & Opciones
- **`GET /api/marketdata/quote/{symbol}`**: Precio real-time (Bid/Ask/Last).
- **`GET /api/marketdata/options/chain/{symbol}`**: Cadena de opciones jerárquica con Griegas en vivo.
- **`GET /api/marketdata/fundamentals/{symbol}`**: Datos fundamentales con predicción de Earnings.
- **`POST /api/marketdata/fundamentals/batch`**: Hidratación masiva de fundamentales (Body: `["AAPL", "MSFT", ...]`).

### 2. Datos Históricos (Candles)
- **`GET /api/marketdata/historical/{symbol}?timeframe={T}&bars={N}`**: Histórico de velas.
- **`GET /api/marketdata/historical/{symbol}/last?timeframe={T}`**: Última vela cerrada.
- **`GET /api/marketdata/historical/{symbol}/current?timeframe={T}`**: Vela en formación (Live).
- **`POST /api/marketdata/historical/batch`**: Descarga masiva de velas para múltiples símbolos.

### 3. Eventos Corporativos (Earnings)
- **`GET /api/marketdata/earnings/{symbol}`**: Historial de reportes y fechas proyectadas.
- **`POST /api/marketdata/earnings/batch`**: Consulta masiva de eventos próximos.

### 4. Metadatos & Diccionario
- **`GET /api/marketdata/timeframes`**: Lista de timeframes soportados (M1, M5, H1, D1, etc.).
- **`GET /api/marketdata/markets`**: Lista de mercados activos (NYSE, NASDAQ, ETF, etc.).
- **`GET /api/marketdata/symbols?markets=NYSE`**: Diccionario de símbolos por mercado.

### 5. Gestión de Órdenes (Trading)
- **`POST /api/marketdata/orders`**: Colocación de Bracket Orders (Entry, TP, SL).
- **`DELETE /api/marketdata/orders/{orderId}`**: Cancelación de orden activa.

### 6. Monitoreo & Salud (DevOps)
- **`GET /api/marketdata/api/health/dxlink/status`**: Estado de la sesión dxLink.
- **`POST /api/marketdata/api/health/dxlink/reconnect`**: Forzar reconexión del WebSocket.
- **`GET /api/marketdata/api/actuator/health`**: Estado general del microservicio.

---

## 📊 Formatos de Datos (DTOs)

### OptionContractDTORespuesta
```json
{
  "symbol": "AAPL  240517C00190000",
  "strikePrice": 190.0,
  "expirationDate": "2024-05-17",
  "optionType": "CALL",
  "delta": 0.542,
  "theoreticalPrice": 4.55
}
```

### FundamentalDataDTORespuesta
```json
{
  "symbol": "AAPL",
  "sharesOutstanding": 15400000000,
  "floatShares": 15300000000,
  "nextEarningsDate": "2024-08-01",
  "tradingHalted": false
}
```

---

## ⚠️ Consideraciones de Consumo
1.  **I18N**: Los endpoints de metadatos soportan el header `Accept-Language` (es/en).
2.  **Batching**: Use los endpoints `/batch` siempre que necesite más de 5 símbolos.
3.  **Halt Status**: Siempre verifique `tradingHalted` antes de enviar una orden agresiva al mercado.
