# 🏭 Producción vs Sandbox - TastyTrade Integration

## ⚠️ CONFIGURACIÓN ACTUAL: PRODUCCIÓN

Este microservicio está configurado **por defecto** para usar el entorno de **PRODUCCIÓN** de TastyTrade.

```
┌─────────────────────────────────────────────────────────────────┐
│                    CONFIGURACIÓN ACTUAL                         │
│                                                                 │
│  🔴 PRODUCCIÓN (LIVE TRADING)                                  │
│                                                                 │
│  • API REST: https://api.tastytrade.com                        │
│  • DxLink WebSocket: wss://tasty.dxfeed.com/realtime          │
│  • REAL MONEY TRADING                                          │
│  • REAL MARKET DATA                                            │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🔴 Entorno de PRODUCCIÓN (Configurado por defecto)

### URLs de Producción

| Servicio | URL | Propósito |
|----------|-----|-----------|
| **TastyTrade REST API** | `https://api.tastytrade.com` | Autenticación OAuth, envío de órdenes |
| **DxLink WebSocket** | `wss://tasty.dxfeed.com/realtime` | Datos de mercado en tiempo real |

### Características

✅ **Datos de mercado reales**: Precios actualizados en tiempo real del mercado bursátil
✅ **Live Trading**: Las órdenes se ejecutan con **dinero real**
✅ **Cuenta real de TastyTrade**: Usa tu cuenta de producción
✅ **Transacciones reales**: Compras y ventas afectan tu saldo real

### ⚠️ ADVERTENCIAS

🚨 **CUIDADO**: Todas las órdenes que envíes serán **ejecutadas con dinero real**
🚨 **RESPONSABILIDAD**: Pérdidas y ganancias son reales
🚨 **VALIDACIÓN**: Aunque hay dry-run, las órdenes finales son reales

### Credenciales Requeridas

Para usar el entorno de producción, necesitas:

1. **Cuenta de TastyTrade** (real, no sandbox)
2. **OAuth 2.0 Credentials** obtenidas de https://developer.tastytrade.com
   - `TT_CLIENT_ID`
   - `TT_CLIENT_SECRET`
   - `TT_REFRESH_TOKEN`
3. **Número de cuenta real** (`TASTYTRADE_ACCOUNT_NUMBER`)

### Configuración en `.env`

```bash
# Credenciales de producción
TT_CLIENT_ID=tu_client_id_de_produccion
TT_CLIENT_SECRET=tu_client_secret_de_produccion
TT_REFRESH_TOKEN=tu_refresh_token_de_produccion
TASTYTRADE_ACCOUNT_NUMBER=5WT00001  # Tu cuenta real

# URL de producción (por defecto)
DXLINK_URL=wss://tasty.dxfeed.com/realtime
```

---

## 🟡 Entorno de SANDBOX/DEMO (Opcional)

Si necesitas **testing sin riesgo**, puedes configurar el entorno sandbox.

### URLs de Sandbox

| Servicio | URL | Propósito |
|----------|-----|-----------|
| **TastyTrade REST API** | `https://api.cert.tastyworks.com` | API de certificación (testing) |
| **DxLink WebSocket** | `wss://demo.dxfeed.com/dxlink-ws` | Datos de mercado demo/simulados |

### Características

✅ **Datos simulados**: Precios de mercado demo (pueden no ser reales)
✅ **Paper Trading**: Órdenes simuladas, no afectan dinero real
✅ **Cuenta sandbox**: Usa una cuenta de prueba
✅ **Sin riesgo**: Ideal para desarrollo y testing

### Configuración en `.env` para Sandbox

```bash
# Credenciales de sandbox (si las tienes)
TT_CLIENT_ID=tu_client_id_de_sandbox
TT_CLIENT_SECRET=tu_client_secret_de_sandbox
TT_REFRESH_TOKEN=tu_refresh_token_de_sandbox
TASTYTRADE_ACCOUNT_NUMBER=cuenta_de_prueba

# URL de sandbox
DXLINK_URL=wss://demo.dxfeed.com/dxlink-ws
```

### Cambiar TastyTrade REST API a Sandbox

Si quieres cambiar también la API REST a sandbox (no solo DxLink), debes modificar:

**Archivo**: `src/main/resources/application.yml`

```yaml
tastytrade:
  # Cambiar a sandbox API
  api-base-url: https://api.cert.tastyworks.com  # En lugar de api.tastytrade.com
```

**NOTA**: TastyTrade no siempre provee credenciales de sandbox públicamente. Verifica con su soporte.

---

## 📊 Comparación: Producción vs Sandbox

| Aspecto | 🔴 Producción | 🟡 Sandbox |
|---------|--------------|-----------|
| **Datos de mercado** | Reales, en tiempo real | Simulados/Demo |
| **Órdenes** | Dinero real | Paper trading |
| **Riesgo** | Alto (pérdidas reales) | Nulo (simulado) |
| **Cuenta** | Cuenta real de TastyTrade | Cuenta de prueba |
| **Propósito** | Trading en vivo | Desarrollo y testing |
| **Credenciales** | OAuth real de producción | OAuth de sandbox (si disponible) |
| **REST API** | `api.tastytrade.com` | `api.cert.tastyworks.com` |
| **DxLink** | `tasty.dxfeed.com/realtime` | `demo.dxfeed.com/dxlink-ws` |

---

## 🔄 Cómo Cambiar Entre Entornos

### Opción 1: Solo cambiar DxLink a Sandbox (mantener API de producción)

Útil si quieres datos demo pero aún usar la API real (solo para consultas, no órdenes).

**Archivo**: `.env`

```bash
# Cambiar solo DxLink a sandbox
DXLINK_URL=wss://demo.dxfeed.com/dxlink-ws
```

### Opción 2: Cambiar todo a Sandbox (DxLink + API REST)

**Paso 1**: Cambiar `.env`

```bash
TT_CLIENT_ID=credenciales_de_sandbox
TT_CLIENT_SECRET=credenciales_de_sandbox
TT_REFRESH_TOKEN=credenciales_de_sandbox
TASTYTRADE_ACCOUNT_NUMBER=cuenta_sandbox

DXLINK_URL=wss://demo.dxfeed.com/dxlink-ws
```

**Paso 2**: Modificar `application.yml`

```yaml
tastytrade:
  api-base-url: https://api.cert.tastyworks.com
```

**Paso 3**: Reiniciar el servicio

```bash
./mvnw spring-boot:run
```

---

## 🎯 Recomendaciones

### Para Desarrollo Local

Si estás desarrollando y no quieres riesgo:

1. **Usa DxLink sandbox** para datos demo
2. **Deshabilita envío de órdenes** en el código (comentar lógica)
3. **Usa el flag de dry-run** siempre (ya implementado)

### Para Testing

1. **Escribe tests unitarios** que no requieran conexión real
2. **Usa mocks** para simular respuestas de TastyTrade
3. **Tests de integración** solo en sandbox
4. **Validación exhaustiva** antes de producción

### Para Producción

1. ✅ **Valida credenciales reales** en `.env`
2. ✅ **Verifica la URL de DxLink**: `wss://tasty.dxfeed.com/realtime`
3. ✅ **Revisa la URL de API**: `https://api.tastytrade.com`
4. ✅ **Monitorea logs** en tiempo real
5. ✅ **Dry-run activado** siempre (primera validación antes de enviar orden real)
6. ✅ **Alertas configuradas** para errores críticos

---

## 🔒 Seguridad en Producción

### Protección de Credenciales

```bash
# .env file (NUNCA commitear a git)
TT_CLIENT_ID=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
TT_CLIENT_SECRET=yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy
TT_REFRESH_TOKEN=zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz

# Verificar que está en .gitignore
cat .gitignore | grep .env
# Output: .env
```

### Variables de Entorno del Sistema (Recomendado para producción)

En lugar de usar `.env` en producción, configura variables de entorno del sistema:

```bash
# En servidor de producción
export TT_CLIENT_ID=xxxxxxxxx
export TT_CLIENT_SECRET=yyyyyyyyy
export TT_REFRESH_TOKEN=zzzzzzzz
export TASTYTRADE_ACCOUNT_NUMBER=5WT00001
export DXLINK_URL=wss://tasty.dxfeed.com/realtime
```

O en AWS (ECS/EC2):
- Usar **AWS Secrets Manager**
- Configurar en **Task Definition** (ECS)
- Variables de entorno en **EC2 User Data**

---

## 📝 Checklist de Verificación Pre-Deploy

Antes de desplegar a producción:

- [ ] ✅ Credenciales de **producción** configuradas en `.env` o variables de entorno
- [ ] ✅ `DXLINK_URL=wss://tasty.dxfeed.com/realtime` (producción)
- [ ] ✅ `api-base-url: https://api.tastytrade.com` en `application.yml`
- [ ] ✅ Tests de integración pasando correctamente
- [ ] ✅ Dry-run habilitado en `TastyTradeRestClient`
- [ ] ✅ Logs configurados en nivel INFO o WARN (no DEBUG)
- [ ] ✅ Monitoreo y alertas configurados
- [ ] ✅ Plan de rollback preparado
- [ ] ✅ Límites de trading configurados (risk management)
- [ ] ⚠️ **Revisión manual de código crítico** (envío de órdenes)

---

## 🆘 Troubleshooting

### Error: "Invalid credentials"

**Causa**: Estás usando credenciales de sandbox en entorno de producción (o viceversa).

**Solución**: Verifica que las credenciales en `.env` correspondan al entorno configurado.

### Error: "WebSocket connection failed"

**Causa**: URL de DxLink incorrecta.

**Solución**:
- Producción: `wss://tasty.dxfeed.com/realtime`
- Sandbox: `wss://demo.dxfeed.com/dxlink-ws`

### Órdenes no se ejecutan en producción

**Causa**: Posiblemente estás en sandbox sin saberlo.

**Solución**: Verificar logs y configuración:

```bash
# Ver logs al iniciar
tail -f logs/marketdata-service.log | grep "TastyTrade"

# Deberías ver:
# INFO TastyTradeProperties - Using production API: https://api.tastytrade.com
# INFO DxLinkConnectionManager - Connecting to wss://tasty.dxfeed.com/realtime
```

---

## 📚 Referencias

- **TastyTrade API Docs**: https://developer.tastytrade.com/
- **DxLink Protocol**: https://demo.dxfeed.com/dxlink-ws/debug/
- **OAuth 2.0 Flow**: https://developer.tastytrade.com/getting-started/

---

## ⚖️ Disclaimer Legal

🚨 **IMPORTANTE**: Este software está configurado para operar con **dinero real** en el entorno de producción.

- Las transacciones son **REALES** y **VINCULANTES**
- Las pérdidas son **REALES** y no reembolsables
- El usuario es **COMPLETAMENTE RESPONSABLE** de todas las operaciones
- No hay garantías de ganancias
- El software se proporciona "AS IS" sin garantías

**Úsalo bajo tu propio riesgo y responsabilidad.**

---

**Documentado para MetradingPlat** - 2026-01-12
