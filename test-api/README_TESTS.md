# 🧪 Tastytrade API - Suite de Pruebas Automatizadas

Script Python profesional con **tests automatizados** para validar la API de Tastytrade.

---

## ✨ Características

- ✅ **8 pruebas automatizadas**
- 🎨 **Output colorizado en consola**
- 📊 **Clases y Enums tipados**
- ⚡ **Resultados instantáneos**
- 📈 **Métricas de performance**

---

## 🚀 Instalación

```bash
pip install requests colorama
```

---

## 🔧 Configuración

Edita `tastytrade_tests.py` y pega tus credenciales:

```python
CLIENT_ID = "642e40fa-95b0-4127-8c92-a097b75aecb8"
CLIENT_SECRET = "0feefc7905b38ffd5fddd205bc2ea0c1c1ee0ee7"
REFRESH_TOKEN = "eyJhbGci..."
```

---

## ▶️ Ejecutar

```bash
python tastytrade_tests.py
```

---

## 📋 Pruebas Incluidas

| # | Prueba | Qué Verifica |
|---|--------|--------------|
| 1 | **OAuth Authentication** | Access token válido |
| 2 | **Get All Active Equities** | Obtiene todos los símbolos |
| 3 | **Filter by NYSE** | Solo símbolos NYSE |
| 4 | **Filter by NASDAQ** | Solo símbolos NASDAQ |
| 5 | **Filter ETFs** | Solo ETFs |
| 6 | **Multiple Exchanges** | NYSE + NASDAQ + AMEX |
| 7 | **Search Symbols** | Búsqueda funciona |
| 8 | **Get DXLink Token** | Token para WebSocket |

---

## 📊 Output Ejemplo

```
======================================================================
TASTYTRADE API - SUITE DE PRUEBAS AUTOMATIZADAS
======================================================================

⏳ Ejecutando: Test 1: OAuth Authentication...
✅ PASS Test 1: OAuth Authentication                       523ms
     Access token obtenido (expira en 900s)

⏳ Ejecutando: Test 2: Get All Active Equities...
✅ PASS Test 2: Get All Active Equities                    1245ms
     Total equities: 8543

⏳ Ejecutando: Test 3: Filter by NYSE...
✅ PASS Test 3: Filter by NYSE                             1198ms
     NYSE: 2847 símbolos

     Primeros 10 símbolos NYSE:
     A        (NYSE        ) [Stock] - Agilent Technologies Inc
     AA       (NYSE        ) [Stock] - Alcoa Corporation
     AAC      (NYSE        ) [Stock] - Ares Acquisition Corp
     ...

======================================================================
RESUMEN DE PRUEBAS
======================================================================

✅ TODAS LAS PRUEBAS PASARON

Total:  8
Passed: 8
Failed: 0

Tiempo total: 6234ms
======================================================================
```

---

## 🎨 Clases Principales

### `TastytradeClient`
Cliente principal para interactuar con la API.

```python
client = TastytradeClient(
    client_id="...",
    client_secret="...",
    refresh_token="...",
    environment=Environment.SANDBOX
)

# Obtener tokens
tokens = client.get_oauth_tokens()

# Obtener símbolos
equities = client.get_active_equities()

# Filtrar por exchange
nyse = client.filter_by_exchange(equities, Exchange.NYSE)

# Buscar símbolos
results = client.search_symbols("AAPL")

# DXLink token para WebSocket
dxlink = client.get_dxlink_token()
```

### Enums

```python
class Environment(Enum):
    SANDBOX = "https://api.cert.tastyworks.com"
    PRODUCTION = "https://api.tastyworks.com"

class Exchange(Enum):
    NYSE = "NYSE"
    NASDAQ = "NASDAQ"
    AMEX = "AMEX"
    NYSE_AMERICAN = "NYSE American"
```

### Modelos de Datos

```python
@dataclass
class Equity:
    symbol: str
    description: str
    listed_market: str
    is_etf: bool
    is_index: bool
    streamer_symbol: Optional[str]

@dataclass
class OAuthTokens:
    access_token: str
    token_type: str
    expires_in: int

@dataclass
class DXLinkToken:
    token: str
    dxlink_url: str
    level: str
```

---

## 🔍 Uso Programático

```python
from tastytrade_tests import TastytradeClient, Environment, Exchange

# Crear cliente
client = TastytradeClient(
    client_id="...",
    client_secret="...",
    refresh_token="...",
    environment=Environment.SANDBOX
)

# Autenticar
tokens = client.get_oauth_tokens()
print(f"Token: {tokens.access_token[:20]}...")

# Obtener símbolos NYSE
all_equities = client.get_active_equities()
nyse = client.filter_by_exchange(all_equities, Exchange.NYSE)

for equity in nyse[:10]:
    print(f"{equity.symbol}: {equity.description}")

# Buscar símbolos
results = client.search_symbols("AAPL")
for result in results:
    print(result)

# DXLink token
dxlink = client.get_dxlink_token()
print(f"WebSocket URL: {dxlink.dxlink_url}")
print(f"Token: {dxlink.token[:30]}...")
```

---

## 📝 Notas

- **OTC**: No hay filtro específico, aparecen mezclados en active equities
- **Access Token**: Válido 15 minutos
- **DXLink Token**: Válido 24 horas
- **WebSocket**: Para datos históricos/tiempo real usa `tastytrade_websocket.py`

---

## 🐛 Troubleshooting

### Error: "No se pudo obtener tokens"
- Verifica que las credenciales sean correctas
- Asegúrate que refresh_token no haya expirado

### Error: "No se obtuvieron equities"
- El access_token pudo haber expirado
- Ejecuta de nuevo el script

### Error: "No se pudo obtener DXLink token"
- Necesitas una cuenta de cliente completa
- En sandbox, completa el proceso de registro

---

## 📚 Archivos Relacionados

- `tastytrade_tests.py` - Este script (tests automatizados)
- `tastytrade_websocket.py` - WebSocket para históricos/tiempo real
- `Tastytrade_Essential.postman_collection.json` - Colección Postman

---

## 🎯 Próximos Pasos

Después de ejecutar estos tests:

1. **Para datos históricos**: Usa `tastytrade_websocket.py` opción 1
2. **Para tiempo real**: Usa `tastytrade_websocket.py` opción 2
3. **Integración**: Importa las clases de este script en tu proyecto

---

**¡Listo! Ejecuta `python tastytrade_tests.py` y verás todas las pruebas en acción.**
