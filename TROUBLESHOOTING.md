# 🔧 Troubleshooting Guide - MarketData Service

Guía de resolución de problemas comunes al ejecutar el servicio.

---

## ⚠️ Problemas de Compilación

### Error: "release version 21 not supported"

**Síntoma:**
```
[ERROR] Fatal error compiling: error: release version 21 not supported
```

**Causa:** Maven está usando una versión de Java anterior a 21.

**Solución:**

#### Windows:
```bash
# Verificar versión de Java que Maven usa
mvnw.cmd -v

# Si muestra Java 17 o inferior, configurar JAVA_HOME
set JAVA_HOME=C:\Program Files\Eclipse Adoptium\jdk-21.0.9.10-hotspot
set PATH=%JAVA_HOME%\bin;%PATH%

# O usar el script de compilación
compile.cmd
```

#### Linux/Mac:
```bash
# Verificar versión
./mvnw -v

# Configurar JAVA_HOME
export JAVA_HOME="/path/to/jdk-21"
export PATH="$JAVA_HOME/bin:$PATH"

# Compilar
./mvnw clean compile -DskipTests
```

---

### Error: "ClassNotFoundException: MarketdataServiceApplication"

**Síntoma:**
```
Error: Could not find or load main class com.metradingplat.marketdata.MarketdataServiceApplication
Caused by: java.lang.ClassNotFoundException
```

**Causa:** El proyecto no está compilado.

**Solución:**
```bash
# Windows
compile.cmd

# O manualmente
mvnw.cmd clean compile -DskipTests
```

---

### Error: "Failed to delete target directory"

**Síntoma:**
```
[ERROR] Failed to clean project: Failed to delete E:\...\target\classes\application-prod.yml.tmp...
```

**Causa:** Archivos temporales bloqueados por el IDE o proceso anterior.

**Solución:**
```bash
# Cerrar el IDE y procesos de Java
# Luego eliminar target manualmente

# Windows
rmdir /S /Q target
mvnw.cmd compile

# Linux/Mac
rm -rf target
./mvnw compile
```

---

## 🔴 Problemas de Ejecución

### Error: "Bean 'tastyTradeRestClient' could not be registered"

**Síntoma:**
```
The bean 'tastyTradeRestClient' ... could not be registered.
A bean with that name has already been defined ...
```

**Causa:** Conflicto de definición de beans (ya resuelto en la versión actual).

**Solución:**
```bash
# Recompilar el proyecto
mvnw.cmd clean compile -DskipTests

# Verificar que TastyTradeRestClient NO tenga @Service
# Solo debe tener @RequiredArgsConstructor y @Slf4j
```

---

### Error: "Failed to configure a DataSource"

**Síntoma:**
```
Failed to configure a DataSource: 'url' attribute is not specified
```

**Causa:** PostgreSQL no está corriendo o las credenciales son incorrectas.

**Solución:**

#### 1. Verificar PostgreSQL:
```bash
# Docker
docker ps | grep postgres

# Si no está corriendo
docker-compose up -d postgres

# Verificar logs
docker logs postgres-container
```

#### 2. Verificar conexión:
```bash
# Test de conexión
psql -h localhost -U postgres -d marketdata
```

#### 3. Verificar configuración en `application-dev.yml`:
```yaml
spring:
  datasource:
    url: jdbc:postgresql://localhost:5432/marketdata
    username: postgres
    password: postgres
```

---

### Error: "Failed to connect to Kafka"

**Síntoma:**
```
Connection to node -1 (localhost/127.0.0.1:9092) could not be established
```

**Causa:** Kafka no está corriendo.

**Solución:**
```bash
# Verificar si Kafka está corriendo
docker ps | grep kafka

# Iniciar Kafka y Zookeeper
docker-compose up -d zookeeper kafka

# Verificar logs
docker logs kafka-container

# Esperar ~30 segundos para que Kafka inicie completamente
```

---

### Error: "Eureka registration failed"

**Síntoma:**
```
Cannot execute request on any known server
```

**Causa:** El servicio de descubrimiento (Eureka) no está disponible.

**Solución:**

**Opción 1:** Iniciar Eureka Server
```bash
# Navegar al directorio de directory-service
cd ../directory-service
mvnw spring-boot:run
```

**Opción 2:** Deshabilitar Eureka para testing local
```yaml
# En application-dev.yml, agregar:
eureka:
  client:
    enabled: false
```

---

## 🔐 Problemas de Autenticación

### Error: "Invalid OAuth credentials"

**Síntoma:**
```
401 Unauthorized - Invalid client credentials
```

**Causa:** Credenciales incorrectas en `.env`.

**Solución:**

1. Verificar que `.env` existe:
```bash
# Windows
dir .env

# Linux/Mac
ls -la .env
```

2. Verificar formato correcto:
```bash
TT_CLIENT_ID=b12b2bd8-9b88-48fc-8cf1-d1625625ea07
TT_CLIENT_SECRET=f09eaab581b16722e7fc7952b2c4b028caf9c125
TT_REFRESH_TOKEN=eyJhbGciOi...
TASTYTRADE_ACCOUNT_NUMBER=5WT00001
```

3. **No usar comillas** en los valores
4. **No usar espacios** alrededor del `=`

---

### Error: ".env file not loaded"

**Síntoma:**
```
Property 'TT_CLIENT_ID' not found
```

**Causa:** DotenvConfig no está cargando el archivo `.env`.

**Solución:**

1. Verificar que existe `META-INF/spring.factories`:
```
src/main/resources/META-INF/spring.factories
```

2. Contenido debe ser:
```
org.springframework.context.ApplicationContextInitializer=\
  com.metradingplat.marketdata.infrastructure.config.DotenvConfig
```

3. Recompilar:
```bash
mvnw.cmd clean compile -DskipTests
```

---

### Error: "refresh_token expired"

**Síntoma:**
```
400 Bad Request - refresh_token is expired or invalid
```

**Causa:** El refresh token ha expirado.

**Solución:**
1. Ir a https://developer.tastytrade.com/
2. Generar un nuevo refresh token
3. Actualizar `.env` con el nuevo token
4. Reiniciar el servicio

---

## 🌐 Problemas de WebSocket

### Error: "Failed to connect to DxLink WebSocket"

**Síntoma:**
```
WebSocket connection failed: Connection refused
```

**Causa:** URL incorrecta o problemas de red.

**Solución:**

1. Verificar URL en `.env`:
```bash
# Producción
DXLINK_URL=wss://tasty.dxfeed.com/realtime

# Sandbox
DXLINK_URL=wss://demo.dxfeed.com/dxlink-ws
```

2. Verificar API quote token:
```bash
# Ver logs al iniciar
tail -f logs/marketdata-service.log | grep "API quote token"
```

3. Verificar firewall/proxy no bloquee WSS

---

### Error: "AUTH_STATE: UNAUTHORIZED"

**Síntoma:**
```
DxLink authentication failed: AUTH_STATE UNAUTHORIZED
```

**Causa:** API quote token inválido o expirado.

**Solución:**
1. El servicio debería renovar automáticamente cada 23 horas
2. Verificar que `TokenRefreshScheduler` esté activo
3. Reiniciar el servicio manualmente
4. Verificar logs:
```bash
grep "TokenRefreshScheduler" logs/marketdata-service.log
```

---

## 📊 Problemas de Datos

### Subscripción no recibe datos

**Síntoma:** Subscripción exitosa pero no llegan eventos.

**Solución:**

1. Verificar que el símbolo existe y está activo:
```bash
# Probar con AAPL (siempre activo)
kafka-console-producer --bootstrap-server localhost:9092 --topic marketdata.commands
{"action": "SUBSCRIBE", "symbol": "AAPL"}
```

2. Verificar logs del WebSocket:
```bash
tail -f logs/marketdata-service.log | grep "Quote\|Trade"
```

3. Verificar que Kafka consumer esté activo:
```bash
kafka-console-consumer --bootstrap-server localhost:9092 --topic marketdata.stream --from-beginning
```

4. Horario del mercado: Verificar que el mercado esté abierto
   - NYSE: 9:30 AM - 4:00 PM ET
   - Fuera de horario: datos limitados

---

### Candles históricos vacíos

**Síntoma:** Query retorna lista vacía `[]`.

**Solución:**

1. Verificar rango de fechas:
```bash
# Debe ser formato ISO-8601
GET /api/marketdata/historical/AAPL?timeframe=M5&from=2026-01-10T00:00:00Z&to=2026-01-11T00:00:00Z
```

2. Verificar que el símbolo tiene datos en ese período
3. El mercado debe haber estado abierto en esas fechas
4. Ver logs para detectar errores:
```bash
grep "CandleEventHandler" logs/marketdata-service.log
```

---

## 🐛 Problemas de Testing

### Tests fallan: "EmbeddedKafka not starting"

**Síntoma:**
```
Failed to start EmbeddedKafka
```

**Solución:**
1. Cerrar cualquier instancia local de Kafka
2. Liberar puertos 9092, 2181
```bash
# Windows - Ver qué usa el puerto
netstat -ano | findstr "9092"

# Matar proceso
taskkill /PID <pid> /F
```

---

### Tests fallan: "Database connection refused"

**Síntoma:**
```
Connection refused: localhost:5432
```

**Solución:**
Los tests usan una base de datos H2 en memoria, NO deberían necesitar PostgreSQL.

Verificar que `@DataJpaTest` use H2:
```java
@DataJpaTest
@AutoConfigureTestDatabase(replace = AutoConfigureTestDatabase.Replace.ANY)
```

---

## 🔧 Herramientas de Diagnóstico

### Ver todas las variables de entorno cargadas

```bash
# Agregar esta línea temporalmente en DotenvConfig
log.info("Environment variables loaded: {}", dotenv.entries().toString());
```

### Ver estado de WebSocket

```bash
# Logs de conexión
grep "DxLinkConnectionManager" logs/marketdata-service.log
```

### Ver estado de tokens

```bash
# Logs de tokens (tokens ocultos por seguridad)
grep "TokenRefreshScheduler\|TastyTradeAuthClient" logs/marketdata-service.log
```

### Verificar beans cargados

```bash
# Agregar a application-dev.yml
logging:
  level:
    org.springframework.context: DEBUG
```

---

## 🆘 Soporte Adicional

Si ninguna de estas soluciones funciona:

1. **Revisar logs completos:**
```bash
tail -f logs/marketdata-service.log
```

2. **Habilitar DEBUG logging:**
```yaml
# En application-dev.yml
logging:
  level:
    com.metradingplat.marketdata: DEBUG
```

3. **Limpiar y recompilar:**
```bash
mvnw.cmd clean install -DskipTests
```

4. **Reiniciar TODO:**
```bash
# Detener servicio
Ctrl+C

# Reiniciar Docker
docker-compose down
docker-compose up -d

# Recompilar
mvnw.cmd clean compile

# Reiniciar servicio
run-dev.cmd
```

---

## 📚 Referencias

- [README.md](README.md) - Guía de inicio
- [PRODUCTION_VS_SANDBOX.md](PRODUCTION_VS_SANDBOX.md) - Configuración de entornos
- [TESTING_README.md](TESTING_README.md) - Guía de testing
- [TastyTrade API Docs](https://developer.tastytrade.com/)

---

**Última actualización:** 2026-01-12
