# 📘 Especificación Técnica: Integración con MarketData Service

Este documento define el contrato de API y los mecanismos de sincronización para que otros microservicios (Orders, UI, Quantitative) consuman datos del ecosistema TastyTrade/dxLink.

## 📡 Arquitectura de Entrega
El servicio opera bajo un modelo **Híbrido de Alta Performance**:
1.  **REST (Pull)**: Para estructuras pesadas o datos históricos (Ej: Cadenas de Opciones, Velas).
2.  **Streaming (Push/Cache)**: Los datos volátiles (Quotes, Greeks, Halts) se suscriben vía WebSocket (dxLink) al primer request y se mantienen en un caché de ultra-baja latencia.

---

## 🛠 Endpoints REST Clave

### 1. Option Chains (Cadenas de Opciones)
**Propósito**: Obtener el árbol jerárquico de contratos para una acción.
- **URL**: `GET /api/marketdata/options/chain/{symbol}`
- **Formato de Entrega**: JSON anidado por fecha de expiración.
- **Griegas**: Incluye Delta, Gamma, Theta, Vega e IV en tiempo real si el streaming ya está activo para ese símbolo.

### 2. Fundamentales Predictivos
**Propósito**: Análisis Cuantitativo y detección de "Low Float".
- **URL**: `GET /api/marketdata/fundamentals/{symbol}`
- **Campos Especiales**:
    - `sharesOutstanding`: Total de acciones para cálculo de Market Cap.
    - `floatShares`: Acciones flotantes (crítico para estrategias de momentum).
    - `nextEarningsDate`: **Predicción Algorítmica** basada en la mediana de ciclos históricos (±90 días).
    - `dividendYield`: Calculado dinámicamente vs el precio Last.

### 3. Real-Time Quotes & Halts
**Propósito**: Ejecución de órdenes y gestión de riesgos.
- **URL**: `GET /api/marketdata/quote/{symbol}`
- **Detección de Halt**: El campo `tradingHalted` se activa automáticamente mediante la heurística de dxLink (`Profile` event) o si el spread Bid-Ask se vuelve anómalo por más de 500ms.

---

## ⚡️ Mecanismo de Suscripción (Under the hood)
Cuando un microservicio pide datos de un símbolo por primera vez:
1.  **Trigger**: El `marketdata-service` detecta que el símbolo no está en el `subscribedSymbols`.
2.  **Sub**: Se envía un mensaje `FEED_SUBSCRIPTION` vía WebSocket.
3.  **Hydration**: Se descarga el snapshot inicial y se empieza a actualizar el `greeksCache` y `fundamentalsCache`.
4.  **Consistencia**: Las peticiones subsiguientes obtienen los datos directamente de la memoria (latencia < 1ms).

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
  "impliedVolatility": 0.22,
  "theoreticalPrice": 4.55,
  "bid": 4.50,
  "ask": 4.60
}
```

## ⚠️ Consideraciones de Consumo
1.  **Rate Limiting**: Las peticiones Batch (`/batch`) deben preferirse sobre múltiples peticiones individuales para evitar el throttling de la API de TastyTrade.
2.  **Griegas**: Si una opción tiene volumen cero, las Griegas pueden venir como `null` hasta que dxLink reciba un tick de precio. El consumidor debe manejar estos nulos con `0.0` por defecto.
3.  **Tokens**: El microservicio maneja automáticamente el refresco de tokens OAuth2; los consumidores no necesitan preocuparse por la autenticación externa.
