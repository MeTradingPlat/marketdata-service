# Guía de Testing - MarketData Service

Esta guía explica cómo ejecutar los tests del servicio `marketdata-service` y probar manualmente la integración con TastyTrade y Kafka.

## 📋 Contenido

1. [Tests Automatizados](#tests-automatizados)
2. [Pruebas Manuales con Kafka](#pruebas-manuales-con-kafka)
3. [Pruebas de API REST](#pruebas-de-api-rest)
4. [Troubleshooting](#troubleshooting)

---

## 🧪 Tests Automatizados

### Pre-requisitos

1. **Configurar credenciales de TastyTrade** en variables de entorno:

```bash
export TT_CLIENT_ID=tu_client_id
export TT_CLIENT_SECRET=tu_client_secret
export TT_REFRESH_TOKEN=tu_refresh_token
export TASTYTRADE_ACCOUNT_NUMBER=tu_numero_cuenta
```

O en Windows PowerShell:

```powershell
$env:TT_CLIENT_ID="tu_client_id"
$env:TT_CLIENT_SECRET="tu_client_secret"
$env:TT_REFRESH_TOKEN="tu_refresh_token"
$env:TASTYTRADE_ACCOUNT_NUMBER="tu_numero_cuenta"
```

> [!TIP] > **Maven Wrapper**: Si no tienes Maven instalado globalmente y el comando `mvn` no es reconocido, usa `.\mvnw` en su lugar (ej: `.\mvnw clean test`).

### Ejecutar Todos los Tests

Si tienes Maven instalado globalmente:

```bash
mvn clean test
```

Si **no** tienes Maven instalado, usa el wrapper incluido en el proyecto:

**En Linux/Mac:**

```bash
./mvnw clean test
```

**En Windows (PowerShell/CMD):**

```powershell
.\mvnw clean test
```

### Ejecutar Tests Específicos

#### 1. Test de Autenticación OAuth 2.0

```bash
mvn test -Dtest=TastyTradeAuthenticationIntegrationTest
```

**Qué valida**:

- ✅ Obtención de access token con refresh_token
- ✅ Obtención de API quote token con access token
- ✅ Renovación automática de tokens expirados

**Output esperado**:

```
✅ Access Token obtenido: eyJhbGciOiJIUzI1NiI...
✅ API Quote Token obtenido
   Token: dxLink_1234567890...
   DxLink URL: wss://tasty.dxfeed.com/realtime
✅ Token renovado correctamente

Tests run: 3, Failures: 0, Errors: 0
```

#### 2. Test de WebSocket DxLink

```bash
mvn test -Dtest=DxLinkWebSocketIntegrationTest
```

**Qué valida**:

- ✅ Conexión exitosa al WebSocket DxLink
- ✅ Suscripción a símbolos (AAPL, SPY)
- ✅ Desuscripción de símbolos

**Output esperado**:

```
✅ WebSocket DxLink conectado correctamente
✅ Suscripción a AAPL registrada correctamente
✅ Desuscripción de SPY exitosa

Tests run: 3, Failures: 0, Errors: 0
```

#### 3. Test de Kafka Listeners

```bash
mvn test -Dtest=KafkaIntegrationTest
```

**Qué valida**:

- ✅ Procesamiento de órdenes desde Kafka
- ✅ Suscripción a símbolos desde Kafka
- ✅ Desuscripción desde Kafka

**Output esperado**:

```
✅ Orden procesada correctamente a través de Kafka
✅ Suscripción a SPY procesada correctamente
✅ Desuscripción de TSLA procesada correctamente

Tests run: 3, Failures: 0, Errors: 0
```

---

## 📨 Pruebas Manuales con Kafka

### Setup Inicial

1. **Iniciar Kafka** (si no está corriendo):

```bash
# Con Docker Compose
docker-compose up -d kafka

# O Kafka local
bin/zookeeper-server-start.sh config/zookeeper.properties
bin/kafka-server-start.sh config/server.properties
```

2. **Crear topics** (si no existen):

```bash
kafka-topics --create --topic orders.commands \
  --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1

kafka-topics --create --topic marketdata.commands \
  --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1

kafka-topics --create --topic orders.updates \
  --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1

kafka-topics --create --topic marketdata.stream \
  --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1
```

3. **Iniciar el servicio**:

```bash
cd marketdata-service
mvn spring-boot:run
```

### Prueba 1: Suscribirse a Datos en Tiempo Real

**Terminal 1** - Consumer para ver datos:

```bash
kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic marketdata.stream \
  --from-beginning
```

**Terminal 2** - Enviar comando de suscripción:

```bash
echo '{"symbol":"AAPL","action":"SUBSCRIBE"}' | \
  kafka-console-producer --bootstrap-server localhost:9092 \
  --topic marketdata.commands
```

**Resultado esperado en Terminal 1**:

```json
{"symbol":"AAPL","lastPrice":178.45,"bid":178.44,"ask":178.46,"volume":1500,"timestamp":"2026-01-12T10:30:45.123Z"}
{"symbol":"AAPL","lastPrice":178.46,"bid":178.45,"ask":178.47,"volume":800,"timestamp":"2026-01-12T10:30:47.456Z"}
...
```

### Prueba 2: Enviar una Orden

**Terminal 1** - Consumer para ver respuestas:

```bash
kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic orders.updates \
  --from-beginning
```

**Terminal 2** - Enviar orden:

```bash
echo '{
  "symbol": "AAPL",
  "action": "BUY_TO_OPEN",
  "quantity": 10,
  "type": "LIMIT",
  "price": 178.00,
  "stopLossPrice": 175.00,
  "takeProfitPrice": 182.00
}' | kafka-console-producer --bootstrap-server localhost:9092 \
  --topic orders.commands
```

**Resultado esperado en Terminal 1**:

```json
{
  "orderId": "123456",
  "status": "Received",
  "receivedAt": "2026-01-12T10:31:00.000Z",
  "complexOrderId": null,
  "rejectReason": null
}
```

### Prueba 3: Desuscribirse

```bash
echo '{"symbol":"AAPL","action":"UNSUBSCRIBE"}' | \
  kafka-console-producer --bootstrap-server localhost:9092 \
  --topic marketdata.commands
```

Los datos de AAPL dejan de aparecer en `marketdata.stream`.

---

## 🌐 Pruebas de API REST

### Endpoint: Obtener Candles Históricos

```bash
curl -X GET "http://localhost:8080/api/marketdata/historical/AAPL?timeframe=M5&from=2026-01-10T09:30:00Z&to=2026-01-12T16:00:00Z" \
  -H "Content-Type: application/json"
```

**Con HTTPie**:

```bash
http GET http://localhost:8080/api/marketdata/historical/AAPL \
  timeframe==M5 \
  from==2026-01-10T09:30:00Z \
  to==2026-01-12T16:00:00Z
```

**Respuesta esperada**:

```json
[
  {
    "symbol": "AAPL",
    "open": 177.50,
    "high": 178.20,
    "low": 177.30,
    "close": 178.00,
    "volume": 125000,
    "timestamp": "2026-01-10T09:30:00Z"
  },
  ...
]
```

### Timeframes Soportados

| Código | Descripción |
| ------ | ----------- |
| M1     | 1 minuto    |
| M5     | 5 minutos   |
| M15    | 15 minutos  |
| M30    | 30 minutos  |
| H1     | 1 hora      |
| H4     | 4 horas     |
| D1     | 1 día       |

---

## 🔍 Verificar Logs

### Ver logs del servicio:

```bash
# Ver logs en tiempo real
tail -f logs/marketdata-service.log

# Filtrar solo errores
tail -f logs/marketdata-service.log | grep ERROR

# Verificar conexión WebSocket
grep "WebSocket\|AUTH_STATE" logs/marketdata-service.log | tail -20

# Verificar órdenes
grep "Order" logs/marketdata-service.log | tail -20
```

### Logs importantes a verificar:

```
[INFO] OAuth access token obtained successfully. Expires in: 900 seconds
[INFO] API quote token obtained successfully. DxLink URL: wss://...
[INFO] WebSocket connection established
[INFO] Received AUTH_STATE: AUTHORIZED
[INFO] Channel opened: 1
[INFO] Added subscription: AAPL - QUOTE
[INFO] Candle saved: AAPL @ 2026-01-12T10:30:00Z
```

---

## 🐛 Troubleshooting

### Error: "OAuth authentication failed"

**Causa**: Credenciales incorrectas

**Solución**:

1. Verifica que las variables de entorno estén configuradas correctamente
2. Ve a https://developer.tastytrade.com y regenera las credenciales
3. Reinicia el servicio después de actualizar las credenciales

### Error: "Connection refused" al conectar a Kafka

**Causa**: Kafka no está corriendo

**Solución**:

```bash
# Verificar si Kafka está corriendo
nc -zv localhost 9092

# Iniciar Kafka
docker-compose up -d kafka
```

### No llegan datos en `marketdata.stream`

**Posibles causas**:

1. **WebSocket desconectado**: Verifica en logs que `AUTH_STATE: AUTHORIZED`
2. **Símbolo no válido**: Prueba con símbolos populares (AAPL, SPY, TSLA)
3. **Mercado cerrado**: Los datos solo llegan durante horario de mercado (9:30am - 4:00pm ET)

**Solución**:

```bash
# Verificar estado de conexión
grep "AUTH_STATE\|WebSocket" logs/marketdata-service.log | tail -10

# Reintentar con SPY
echo '{"symbol":"SPY","action":"SUBSCRIBE"}' | \
  kafka-console-producer --bootstrap-server localhost:9092 \
  --topic marketdata.commands
```

### Tests fallan: "TT_CLIENT_ID not found"

**Causa**: Variables de entorno no configuradas

**Solución**:

```bash
# En Linux/Mac
export TT_CLIENT_ID=tu_client_id
export TT_CLIENT_SECRET=tu_client_secret
export TT_REFRESH_TOKEN=tu_refresh_token

# En Windows PowerShell
$env:TT_CLIENT_ID="tu_client_id"
$env:TT_CLIENT_SECRET="tu_client_secret"
$env:TT_REFRESH_TOKEN="tu_refresh_token"

# Verificar
echo $TT_CLIENT_ID  # Linux/Mac
echo $env:TT_CLIENT_ID  # Windows
```

---

## ✅ Checklist de Pruebas

Antes de considerar el servicio listo:

- [ ] Tests automatizados pasan exitosamente
- [ ] Autenticación OAuth 2.0 funciona
- [ ] WebSocket DxLink se conecta y mantiene conexión
- [ ] Suscripción a símbolos vía Kafka funciona
- [ ] Datos en tiempo real llegan a `marketdata.stream`
- [ ] Órdenes se envían correctamente
- [ ] Respuestas de órdenes llegan a `orders.updates`
- [ ] API REST de candles históricos funciona
- [ ] Caché en PostgreSQL funciona
- [ ] Token se renueva automáticamente cada 15 minutos
- [ ] Logs no muestran errores críticos

---

## 📚 Recursos Adicionales

- [TastyTrade API Documentation](https://developer.tastytrade.com/)
- [DxLink WebSocket Protocol](https://demo.dxfeed.com/dxlink-ws/debug/)
- [Apache Kafka Quickstart](https://kafka.apache.org/quickstart)
- [Spring Boot Testing](https://spring.io/guides/gs/testing-web/)

---

**¡Tu microservicio está listo para testing!** 🚀

Para cualquier pregunta o problema, revisa los logs en `logs/marketdata-service.log` o contacta al equipo de desarrollo.
