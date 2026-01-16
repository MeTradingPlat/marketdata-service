# MarketData Service

Microservicio de gestión de datos de mercado con integración a TastyTrade DxLink WebSocket para datos en tiempo real y órdenes.

---

## Arquitectura del Sistema

Este servicio forma parte de la arquitectura de microservicios de MetradingPlat:

- **Directory (Eureka)**: Servicio de descubrimiento en puerto 8761
- **Gateway**: API Gateway en puerto 8080
- **MarketData Service**: Este servicio en puerto 8082

El servicio se registra automáticamente en Eureka y solo acepta peticiones que pasen por el Gateway (header `X-Gateway-Passed`).

---

## Requisitos Previos

- **Java 21** (JDK 21.0.9 o superior)
- **Maven 3.9+** (incluido via wrapper `mvnw.cmd`)
- **PostgreSQL** (Docker en producción)
- **Apache Kafka** (Docker en producción)
- **Credenciales de TastyTrade** (OAuth 2.0)

---

## Desarrollo Local

### 1. Configurar Variables de Entorno

Para desarrollo local, configura las siguientes variables de entorno en tu sistema:

```bash
# Windows (PowerShell)
$env:TT_CLIENT_ID="tu_client_id"
$env:TT_CLIENT_SECRET="tu_client_secret"
$env:TT_REFRESH_TOKEN="tu_refresh_token"
$env:TASTYTRADE_ACCOUNT_NUMBER="tu_numero_cuenta"

# Linux/Mac
export TT_CLIENT_ID="tu_client_id"
export TT_CLIENT_SECRET="tu_client_secret"
export TT_REFRESH_TOKEN="tu_refresh_token"
export TASTYTRADE_ACCOUNT_NUMBER="tu_numero_cuenta"
```

### 2. Iniciar Servicios de Infraestructura

Asegúrate de tener corriendo:
- **Directory (Eureka)** en `localhost:8761`
- **PostgreSQL** en `localhost:5432`
- **Kafka** en `localhost:9092`

### 3. Compilar el Proyecto

```bash
# Windows
mvnw.cmd clean compile -DskipTests

# Linux/Mac
./mvnw clean compile -DskipTests
```

### 4. Ejecutar el Servicio

```bash
# Windows
mvnw.cmd spring-boot:run

# Linux/Mac
./mvnw spring-boot:run
```

El servicio iniciará en el puerto 8082 y se registrará en Eureka.

---

## Despliegue en Producción (GitHub Actions)

El servicio usa GitHub Actions para CI/CD automatizado.

### GitHub Secrets Requeridos

Configura los siguientes secrets en tu repositorio u organización de GitHub:

| Secret | Descripción |
|--------|-------------|
| `DOCKER_USERNAME` | Usuario de DockerHub |
| `DOCKER_PASSWORD` | Password de DockerHub |
| `POSTGRES_PASSWORD` | Password de PostgreSQL |
| `TT_CLIENT_ID` | TastyTrade OAuth Client ID |
| `TT_CLIENT_SECRET` | TastyTrade OAuth Client Secret |
| `TT_REFRESH_TOKEN` | TastyTrade OAuth Refresh Token |
| `TASTYTRADE_ACCOUNT_NUMBER` | Número de cuenta TastyTrade |
| `DXLINK_URL` | URL del WebSocket DxLink |

### Workflows

- **CI** ([.github/workflows/ci.yml](.github/workflows/ci.yml)): Se ejecuta en cada push a `master`. Compila el proyecto y publica la imagen Docker.

- **CD** ([.github/workflows/cd.yml](.github/workflows/cd.yml)): Se ejecuta automáticamente después de CI exitoso. Despliega en el servidor self-hosted con toda la infraestructura (Kafka, PostgreSQL).

### Infraestructura Desplegada

El CD workflow despliega automáticamente:

| Contenedor | Puerto | Red |
|------------|--------|-----|
| `zookeeper` | 2181 | metradingplat-network |
| `kafka` | 9092 | metradingplat-network |
| `postgres-marketdata` | 5434 | metradingplat-network |
| `marketdata-service` | 8082 | metradingplat-network |

---

## Configuración

### Perfiles Disponibles

- **dev**: Desarrollo local
  - PostgreSQL: `localhost:5432`
  - Kafka: `localhost:9092`
  - Eureka: `localhost:8761`

- **prod**: Producción (Docker)
  - PostgreSQL: `postgres-marketdata:5432`
  - Kafka: `kafka:29092`
  - Eureka: `directory:8761`

### Variables de Entorno

| Variable | Descripción | Default |
|----------|-------------|---------|
| `TT_CLIENT_ID` | TastyTrade OAuth Client ID | Requerido |
| `TT_CLIENT_SECRET` | TastyTrade OAuth Client Secret | Requerido |
| `TT_REFRESH_TOKEN` | TastyTrade OAuth Refresh Token | Requerido |
| `TASTYTRADE_ACCOUNT_NUMBER` | Número de cuenta TastyTrade | Requerido |
| `DXLINK_URL` | URL del WebSocket DxLink | `wss://tasty.dxfeed.com/realtime` |
| `DB_HOST` | Host PostgreSQL | `localhost` |
| `POSTGRES_USER` | Usuario PostgreSQL | `user_marketdata` |
| `POSTGRES_PASSWORD` | Password PostgreSQL | Requerido en prod |
| `KAFKA_BOOTSTRAP_SERVERS` | Servidores Kafka | `localhost:9092` |
| `EUREKA_HOST` | Host de Eureka | `localhost` |

---

## Arquitectura Interna

### Componentes Principales

1. **TastyTrade Integration**
   - OAuth 2.0 Authentication
   - DxLink WebSocket para datos en tiempo real
   - REST API para envío de órdenes

2. **Gateway Header Filter**
   - Bloquea peticiones que no pasen por el Gateway
   - Requiere header `X-Gateway-Passed: true`

3. **Kafka Listeners**
   - `orders.commands` - Recibe comandos de órdenes
   - `marketdata.commands` - Recibe comandos de suscripción
   - `orders.updates` - Publica actualizaciones de órdenes
   - `marketdata.stream` - Publica datos en tiempo real

4. **REST API**
   - `GET /api/marketdata/historical/{symbol}` - Candles históricos

5. **PostgreSQL**
   - Caché de datos históricos
   - Persistencia de candles

### Flujo de Datos

```
Kafka (commands) → Use Cases → TastyTrade Facade
                                    ↓
                    ┌───────────────┴───────────────┐
                    ↓                               ↓
            DxLink WebSocket                  REST API Client
            (Real-time data)                  (Orders)
                    ↓                               ↓
            Event Handlers ──────────────→ Kafka Producer
                    ↓                               ↓
                PostgreSQL                   orders.updates
                    ↓
            marketdata.stream
```

---

## Topics de Kafka

### INPUT (Consumers)

- `orders.commands` - Comandos de órdenes
  ```json
  {
    "symbol": "AAPL",
    "action": "BUY_TO_OPEN",
    "quantity": 10,
    "type": "LIMIT",
    "price": 178.0
  }
  ```

- `marketdata.commands` - Comandos de suscripción
  ```json
  {
    "symbol": "AAPL",
    "action": "SUBSCRIBE"
  }
  ```

### OUTPUT (Producers)

- `orders.updates` - Actualizaciones de órdenes
  ```json
  {
    "orderId": "123456",
    "status": "Received",
    "receivedAt": "2026-01-12T10:30:00Z"
  }
  ```

- `marketdata.stream` - Datos en tiempo real
  ```json
  {
    "symbol": "AAPL",
    "lastPrice": 178.45,
    "bid": 178.44,
    "ask": 178.46,
    "volume": 1500,
    "timestamp": "2026-01-12T10:30:45.123Z"
  }
  ```

---

## Troubleshooting

### Error: "OAuth authentication failed"
- Verifica que las variables de entorno estén configuradas correctamente
- En producción, verifica los GitHub Secrets

### Error: "Connection refused" (Kafka)
- En desarrollo: Verifica que Kafka esté corriendo localmente
- En producción: El CD workflow inicia Kafka automáticamente

### Error: "Access denied: Request must pass through API Gateway"
- Las peticiones deben pasar por el Gateway en puerto 8080
- El Gateway agrega el header `X-Gateway-Passed: true`

### Error: "WebSocket disconnected"
- El token se renueva automáticamente cada 15 minutos
- Verifica los logs para más detalles

---

## Features

- OAuth 2.0 Authentication con TastyTrade
- WebSocket persistente con DxLink
- Auto-renovación de tokens
- Suscripción a datos en tiempo real (Quote, Trade, Candle)
- Envío de órdenes con validación dry-run
- Caché inteligente en PostgreSQL
- Publicación a Kafka en tiempo real
- CI/CD automatizado con GitHub Actions
- Despliegue containerizado con Docker
- Integración con Eureka para service discovery
- Filtro de seguridad Gateway-only

---

**Desarrollado para MetradingPlat**
