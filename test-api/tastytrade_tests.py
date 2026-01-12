"""
Tastytrade API - Versión Corregida
===================================
Requisitos: pip install requests websocket-client colorama
"""

import requests
import json
import time
import logging
import os
from datetime import datetime, timedelta
from typing import List, Optional
from colorama import init, Fore
from dataclasses import dataclass

init(autoreset=True)

# ============================================================
# CONFIGURACIÓN DE LOGGING
# ============================================================

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    handlers=[
        logging.FileHandler('tastytrade_debug.log', encoding='utf-8'),
    ]
)
logger = logging.getLogger(__name__)

# ============================================================
# CONFIGURACIÓN
# ============================================================

SANDBOX_URL = "https://api.cert.tastyworks.com"
PRODUCTION_URL = "https://api.tastyworks.com"
CONFIG_FILE = "config.json"
TOKEN_CACHE_FILE = "dxlink_token_cache.json"

# ============================================================
# FUNCIONES DE CONFIGURACIÓN
# ============================================================

def save_config(client_id: str, client_secret: str, refresh_token: str, use_production: bool):
    """Guardar credenciales en config.json"""
    config = {
        "client_id": client_id,
        "client_secret": client_secret,
        "refresh_token": refresh_token,
        "use_production": use_production
    }
    try:
        with open(CONFIG_FILE, 'w') as f:
            json.dump(config, f, indent=2)
        logger.info("Credenciales guardadas")
        print(f"{Fore.GREEN}✓ Credenciales guardadas en config.json\n")
    except Exception as e:
        logger.error(f"Error guardando config: {e}")

def load_config() -> Optional[dict]:
    """Cargar credenciales desde config.json"""
    if not os.path.exists(CONFIG_FILE):
        logger.info("No existe config.json")
        return None
    
    try:
        with open(CONFIG_FILE, 'r') as f:
            config = json.load(f)
        logger.info("Credenciales cargadas desde config.json")
        print(f"{Fore.GREEN}✓ Credenciales cargadas desde config.json\n")
        return config
    except Exception as e:
        logger.error(f"Error cargando config: {e}")
        return None

def save_dxlink_token(token: str, url: str):
    """Guardar DXLink token con timestamp"""
    cache = {
        "token": token,
        "url": url,
        "timestamp": datetime.now().isoformat()
    }
    try:
        with open(TOKEN_CACHE_FILE, 'w') as f:
            json.dump(cache, f, indent=2)
        logger.info("DXLink token guardado en cache")
        print(f"{Fore.GREEN}✓ DXLink token guardado en cache\n")
    except Exception as e:
        logger.error(f"Error guardando token: {e}")

def load_dxlink_token() -> Optional[dict]:
    """Cargar DXLink token desde cache si no ha expirado (24 horas)"""
    if not os.path.exists(TOKEN_CACHE_FILE):
        logger.info("No existe cache de token")
        return None
    
    try:
        with open(TOKEN_CACHE_FILE, 'r') as f:
            cache = json.load(f)
        
        # Verificar si el token ha expirado (24 horas)
        token_time = datetime.fromisoformat(cache["timestamp"])
        now = datetime.now()
        time_diff = now - token_time
        
        # Si han pasado más de 24 horas, el token expiró
        if time_diff.total_seconds() > (24 * 60 * 60):
            logger.info(f"Token expirado (creado hace {time_diff})")
            print(f"{Fore.YELLOW}⚠️  Token expirado, se solicitará uno nuevo\n")
            return None
        
        hours_left = 24 - (time_diff.total_seconds() / 3600)
        logger.info(f"Token válido, expira en {hours_left:.1f} horas")
        print(f"{Fore.GREEN}✓ Token válido desde cache (expira en {hours_left:.1f} horas)\n")
        return cache
        
    except Exception as e:
        logger.error(f"Error cargando token cache: {e}")
        return None

# ============================================================
# MODELOS
# ============================================================

@dataclass
class Equity:
    symbol: str
    description: str
    listed_market: str
    is_etf: bool
    streamer_symbol: str

@dataclass
class Candle:
    time: str
    open: float
    high: float
    low: float
    close: float
    volume: float

@dataclass
class Quote:
    timestamp: str
    symbol: str
    bid_price: float
    bid_size: int
    bid_exchange: str
    ask_price: float
    ask_size: int
    ask_exchange: str
    spread: float

# ============================================================
# CLIENTE
# ============================================================

class TastytradeClient:
    """Cliente para Tastytrade API"""
    
    def __init__(self, client_id: str, client_secret: str, refresh_token: str, use_production: bool = False):
        self.client_id = client_id
        self.client_secret = client_secret
        self.refresh_token = refresh_token
        self.base_url = PRODUCTION_URL if use_production else SANDBOX_URL
        self.access_token: Optional[str] = None
        self.dxlink_token: Optional[str] = None
        self.dxlink_url: Optional[str] = None
        
        logger.info(f"Cliente inicializado: {'Production' if use_production else 'Sandbox'}")
    
    def authenticate(self) -> bool:
        """Autenticar"""
        print(f"{Fore.CYAN}🔐 Autenticando...")
        logger.info("Iniciando autenticación")
        
        url = f"{self.base_url}/oauth/token"
        data = {
            "grant_type": "refresh_token",
            "refresh_token": self.refresh_token,
            "client_id": self.client_id,
            "client_secret": self.client_secret
        }
        
        try:
            response = requests.post(url, data=data, timeout=10)
            response.raise_for_status()
            
            json_data = response.json()
            self.access_token = json_data["access_token"]
            
            logger.info("Autenticación exitosa")
            print(f"{Fore.GREEN}✅ Autenticado correctamente")
            return True
            
        except Exception as e:
            logger.error(f"Error autenticación: {e}")
            print(f"{Fore.RED}❌ Error: {e}")
            return False
    
    def get_dxlink_token(self) -> bool:
        """Obtener DXLink token (usa cache si no ha expirado)"""
        logger.info("Verificando DXLink token")
        
        # Intentar cargar token desde cache
        cached = load_dxlink_token()
        if cached:
            self.dxlink_token = cached["token"]
            self.dxlink_url = cached["url"]
            return True
        
        # Si no hay cache o expiró, pedir nuevo token
        print(f"{Fore.CYAN}🔑 Solicitando nuevo DXLink token...")
        logger.info("Solicitando nuevo DXLink token")
        
        if not self.access_token:
            logger.error("No hay access token")
            print(f"{Fore.RED}❌ Debes autenticar primero")
            return False
        
        url = f"{self.base_url}/api-quote-tokens"
        headers = {"Authorization": f"Bearer {self.access_token}"}
        
        try:
            response = requests.get(url, headers=headers, timeout=10)
            response.raise_for_status()
            
            data = response.json()["data"]
            self.dxlink_token = data["token"]
            self.dxlink_url = data["dxlink-url"]
            
            # Guardar token en cache
            save_dxlink_token(self.dxlink_token, self.dxlink_url)
            
            logger.info(f"Nuevo DXLink token obtenido: {self.dxlink_url}")
            print(f"{Fore.GREEN}✅ Nuevo DXLink token obtenido (válido 24 horas)")
            return True
            
        except Exception as e:
            logger.error(f"Error obteniendo DXLink token: {e}")
            print(f"{Fore.RED}❌ Error: {e}")
            return False
    
    def get_all_symbols(self) -> List[Equity]:
        """Obtener todos los símbolos mediante REST API"""
        print(f"\n{Fore.CYAN}{'='*60}")
        print(f"{Fore.CYAN}📊 FUNCIÓN 1: OBTENER TODOS LOS SÍMBOLOS (REST API)")
        print(f"{Fore.CYAN}{'='*60}\n")
        logger.info("Obteniendo símbolos")
        
        if not self.access_token:
            return []
        
        print(f"{Fore.CYAN}🔄 Obteniendo símbolos con paginación...")
        
        url = f"{self.base_url}/instruments/equities/active"
        headers = {"Authorization": f"Bearer {self.access_token}"}
        
        all_equities = []
        page_offset = 0
        total_pages = None
        
        try:
            while True:
                params = {"per-page": 5000, "page-offset": page_offset}
                
                logger.info(f"Solicitando página {page_offset + 1}")
                response = requests.get(url, headers=headers, params=params, timeout=60)
                response.raise_for_status()
                
                json_data = response.json()
                items = json_data["data"]["items"]
                pagination = json_data.get("pagination", {})
                
                if total_pages is None:
                    total_pages = pagination.get("total-pages", 1)
                    total_items = pagination.get("total-items", len(items))
                    print(f"{Fore.CYAN}📊 Total: {total_items} símbolos en {total_pages} página(s)")
                    logger.info(f"Total: {total_items} símbolos, {total_pages} páginas")
                
                for item in items:
                    equity = Equity(
                        symbol=item["symbol"],
                        description=item.get("description", ""),
                        listed_market=item.get("listed-market", "UNKNOWN"),
                        is_etf=item.get("is-etf", False),
                        streamer_symbol=item.get("streamer-symbol", item["symbol"])
                    )
                    all_equities.append(equity)
                
                current_page = page_offset + 1
                print(f"{Fore.GREEN}✓ Página {current_page}/{total_pages} - {len(all_equities)} símbolos obtenidos")
                
                if current_page >= total_pages:
                    break
                
                page_offset += 1
            
            logger.info(f"Total obtenido: {len(all_equities)} símbolos")
            print(f"\n{Fore.GREEN}✅ Total obtenido: {len(all_equities)} símbolos\n")
            
            # Resumen
            exchanges = {}
            for e in all_equities:
                exchanges[e.listed_market] = exchanges.get(e.listed_market, 0) + 1
            
            print(f"{Fore.WHITE}📊 Resumen por Exchange:")
            for exchange, count in sorted(exchanges.items(), key=lambda x: x[1], reverse=True):
                print(f"   {Fore.CYAN}{exchange:<15} {count:>6} símbolos")
            print()
            
            return all_equities
            
        except Exception as e:
            logger.error(f"Error obteniendo símbolos: {e}")
            print(f"{Fore.RED}❌ Error: {e}")
            return all_equities
    
    def get_historical_candles(self, symbol: str, minutes: int = 15, interval: str = "1m") -> List[Candle]:
        """
        Obtener datos históricos OHLC mediante WebSocket
        
        Args:
            symbol: Símbolo a consultar (ej: 'AAPL')
            minutes: Cantidad de minutos hacia atrás (ej: 15 para últimos 15 minutos)
            interval: Intervalo de las velas (ej: '1m', '5m', '15m', '1h', '1d')
        
        Returns:
            Lista de velas OHLC
        """
        print(f"\n{Fore.CYAN}{'='*60}")
        print(f"{Fore.CYAN}📈 FUNCIÓN 2: DATOS HISTÓRICOS OHLC (WebSocket TimeSeries)")
        print(f"{Fore.CYAN}{'='*60}\n")
        logger.info(f"Históricos: {symbol}, {minutes} minutos, {interval}")
        
        if not self.dxlink_token or not self.dxlink_url:
            print(f"{Fore.RED}❌ Debes obtener DXLink token primero")
            return []
        
        print(f"{Fore.CYAN}📊 Símbolo: {symbol}")
        print(f"{Fore.CYAN}📅 Período: 10:00 AM - 10:15 AM (15 minutos fijos)")
        print(f"{Fore.CYAN}⏰ Intervalo: {interval}")
        print(f"{Fore.CYAN}🔌 Conectando a WebSocket...\n")
        
        try:
            import websocket as ws_module
        except ImportError:
            print(f"{Fore.RED}❌ Instala: pip install websocket-client")
            return []
        
        candles = []
        start_time = {"value": time.time()}
        
        def on_message(ws, message):
            try:
                data = json.loads(message)
                msg_type = data.get("type")
                logger.debug(f"RX: {msg_type}")
                
                if msg_type == "SETUP":
                    logger.info("SETUP recibido")
                    auth_msg = {"type": "AUTH", "channel": 0, "token": self.dxlink_token}
                    ws.send(json.dumps(auth_msg))
                    
                elif msg_type == "AUTH_STATE":
                    if data.get("state") == "AUTHORIZED":
                        print(f"{Fore.GREEN}✓ Autorizado")
                        logger.info("Autorizado")
                        channel_msg = {
                            "type": "CHANNEL_REQUEST",
                            "channel": 1,
                            "service": "FEED",
                            "parameters": {"contract": "AUTO"}
                        }
                        ws.send(json.dumps(channel_msg))
                    
                elif msg_type == "CHANNEL_OPENED":
                    print(f"{Fore.GREEN}✓ Canal abierto")
                    logger.info("Canal abierto")
                    
                    # Configurar feed para Candle
                    feed_setup = {
                        "type": "FEED_SETUP",
                        "channel": 1,
                        "acceptDataFormat": "COMPACT",
                        "acceptEventFields": {
                            "Candle": ["eventSymbol", "eventTime", "eventFlags", "index", 
                                      "time", "sequence", "count", "open", "high", "low", 
                                      "close", "volume", "vwap", "bidVolume", "askVolume"]
                        }
                    }
                    ws.send(json.dumps(feed_setup))
                    
                elif msg_type == "FEED_CONFIG":
                    print(f"{Fore.GREEN}✓ Feed configurado")
                    logger.info("Feed configurado")
                    
                    # Usar tiempo fijo: 10:00 AM a 10:15 AM de hoy
                    today = datetime.now().replace(hour=10, minute=0, second=0, microsecond=0)
                    from_time = int(today.timestamp() * 1000)
                    to_time = int((today + timedelta(minutes=15)).timestamp() * 1000)
                    
                    # Formato según DXFeed: SYMBOL{=INTERVAL,fromTime=TIMESTAMP,toTime=TIMESTAMP}
                    candle_symbol = f"{symbol}{{={interval},fromTime={from_time},toTime={to_time}}}"
                    
                    print(f"{Fore.CYAN}📈 Solicitando: {candle_symbol}")
                    print(f"{Fore.CYAN}   Desde: {datetime.fromtimestamp(from_time/1000).strftime('%Y-%m-%d %H:%M:%S')}")
                    print(f"{Fore.CYAN}   Hasta: {datetime.fromtimestamp(to_time/1000).strftime('%Y-%m-%d %H:%M:%S')}")
                    print(f"{Fore.YELLOW}   (Período fijo de 15 minutos para prueba)\n")
                    logger.info(f"Suscribiendo a Candle: {candle_symbol}")
                    
                    # Suscribirse a candles históricos
                    subscribe_msg = {
                        "type": "FEED_SUBSCRIPTION",
                        "channel": 1,
                        "add": [{"type": "Candle", "symbol": candle_symbol}]
                    }
                    ws.send(json.dumps(subscribe_msg))
                    logger.info(f"FEED_SUBSCRIPTION enviado")
                    
                elif msg_type == "FEED_DATA":
                    feed_data = data.get("data", [])
                    logger.info(f"FEED_DATA recibido: {feed_data}")
                    
                    if feed_data and feed_data[0] == "Candle":
                        print(f"{Fore.CYAN}📊 Procesando datos Candle...")
                        raw_data = feed_data[1:] if len(feed_data) > 1 else []
                        logger.info(f"Candles raw data: {len(raw_data)} items - {raw_data}")
                        
                        for idx, item in enumerate(raw_data):
                            if isinstance(item, list) and len(item) >= 11:
                                try:
                                    # Formato COMPACT: [eventSymbol, eventTime, eventFlags, index, 
                                    #                   time, sequence, count, open, high, low, close, volume, ...]
                                    time_val = item[4] if len(item) > 4 else None
                                    open_val = item[7] if len(item) > 7 else None
                                    high_val = item[8] if len(item) > 8 else None
                                    low_val = item[9] if len(item) > 9 else None
                                    close_val = item[10] if len(item) > 10 else None
                                    volume_val = item[11] if len(item) > 11 else None
                                    
                                    if time_val and close_val and close_val > 0:
                                        candle = Candle(
                                            time=datetime.fromtimestamp(time_val / 1000).strftime("%Y-%m-%d %H:%M:%S"),
                                            open=float(open_val) if open_val else 0.0,
                                            high=float(high_val) if high_val else 0.0,
                                            low=float(low_val) if low_val else 0.0,
                                            close=float(close_val),
                                            volume=float(volume_val) if volume_val else 0.0
                                        )
                                        candles.append(candle)
                                        logger.debug(f"Candle {idx+1}: {candle.time} - C:{candle.close}")
                                
                                except (ValueError, TypeError, IndexError) as e:
                                    logger.error(f"Error parseando candle {idx}: {e}")
                        
                        if len(candles) > 0:
                            print(f"{Fore.GREEN}✓ {len(candles)} velas OHLC recibidas hasta ahora")
                            logger.info(f"Total candles acumulados: {len(candles)}")
                        
                        # Cerrar después de timeout o si recibimos datos suficientes
                        if time.time() - start_time["value"] > 15:
                            logger.info(f"Timeout alcanzado: {len(candles)} candles obtenidos")
                            ws.close()
            
            except Exception as e:
                logger.error(f"Error en on_message: {e}")
                print(f"{Fore.RED}❌ Error: {e}")
        
        def on_error(ws, error):
            logger.error(f"WebSocket error: {error}")
        
        def on_close(ws, close_status_code, close_msg):
            logger.info("WebSocket cerrado")
        
        def on_open(ws):
            logger.info("WebSocket abierto")
            setup_msg = {
                "type": "SETUP",
                "channel": 0,
                "version": "0.1-DXF-JS/0.3.0",
                "keepaliveTimeout": 60,
                "acceptKeepaliveTimeout": 60
            }
            ws.send(json.dumps(setup_msg))
        
        # Conectar
        ws = ws_module.WebSocketApp(
            self.dxlink_url,
            on_open=on_open,
            on_message=on_message,
            on_error=on_error,
            on_close=on_close
        )
        
        import threading
        ws_thread = threading.Thread(target=ws.run_forever)
        ws_thread.daemon = True
        ws_thread.start()
        ws_thread.join(timeout=20)
        ws.close()
        
        # Ordenar por tiempo
        candles.sort(key=lambda c: c.time)
        
        # Guardar
        if candles:
            filename = f"historical_{symbol}_{minutes}min_{interval}.txt"
            with open(filename, 'w', encoding='utf-8') as f:
                f.write(f"DATOS HISTÓRICOS OHLC - {symbol}\n")
                f.write(f"Período: 10:00 AM - 10:15 AM (15 minutos)\n")
                f.write(f"Intervalo: {interval}\n")
                f.write(f"Total: {len(candles)} velas\n")
                f.write(f"Fecha: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
                f.write("="*90 + "\n\n")
                f.write(f"{'Fecha/Hora':<20} {'Open':>10} {'High':>10} {'Low':>10} {'Close':>10} {'Volume':>15}\n")
                f.write("-"*90 + "\n")
                
                for candle in candles:
                    f.write(f"{candle.time:<20} "
                           f"${candle.open:>9.2f} "
                           f"${candle.high:>9.2f} "
                           f"${candle.low:>9.2f} "
                           f"${candle.close:>9.2f} "
                           f"{int(candle.volume):>15,}\n")
            
            logger.info(f"Guardado en {filename}")
            print(f"\n{Fore.GREEN}✅ Guardado: {filename}")
            print(f"{Fore.GREEN}   Primera vela: {candles[0].time} - Close: ${candles[0].close:.2f}")
            print(f"{Fore.GREEN}   Última vela:  {candles[-1].time} - Close: ${candles[-1].close:.2f}\n")
        else:
            print(f"\n{Fore.YELLOW}⚠️  No se recibieron velas OHLC")
            print(f"{Fore.YELLOW}   Continuando con streaming en tiempo real...\n")
            logger.warning("No se recibieron candles")
        
        return candles
    
    def get_realtime_quotes(self, symbol: str, duration_seconds: int = 30) -> List[Quote]:
        """
        Obtener quotes en tiempo real (bid/ask) mediante WebSocket
        
        Args:
            symbol: Símbolo a consultar (ej: 'AAPL')
            duration_seconds: Duración del streaming en segundos
        
        Returns:
            Lista de quotes con bid/ask
        """
        print(f"\n{Fore.CYAN}{'='*60}")
        print(f"{Fore.CYAN}📡 FUNCIÓN 3: STREAMING EN TIEMPO REAL (WebSocket Quote)")
        print(f"{Fore.CYAN}{'='*60}\n")
        logger.info(f"Streaming: {symbol}, {duration_seconds}s")
        
        if not self.dxlink_token or not self.dxlink_url:
            print(f"{Fore.RED}❌ Debes obtener DXLink token primero")
            return []
        
        print(f"{Fore.CYAN}📊 Símbolo: {symbol}")
        print(f"{Fore.CYAN}⏱️  Duración: {duration_seconds}s")
        print(f"{Fore.CYAN}🔌 Conectando...\n")
        
        try:
            import websocket as ws_module
        except ImportError:
            print(f"{Fore.RED}❌ Instala: pip install websocket-client")
            return []
        
        quotes = []
        start_time = time.time()
        
        def on_message(ws, message):
            try:
                data = json.loads(message)
                msg_type = data.get("type")
                
                if msg_type == "SETUP":
                    auth_msg = {"type": "AUTH", "channel": 0, "token": self.dxlink_token}
                    ws.send(json.dumps(auth_msg))
                    
                elif msg_type == "AUTH_STATE":
                    if data.get("state") == "AUTHORIZED":
                        print(f"{Fore.GREEN}✓ Autorizado")
                        channel_msg = {
                            "type": "CHANNEL_REQUEST",
                            "channel": 1,
                            "service": "FEED",
                            "parameters": {"contract": "AUTO"}
                        }
                        ws.send(json.dumps(channel_msg))
                    
                elif msg_type == "CHANNEL_OPENED":
                    print(f"{Fore.GREEN}✓ Canal abierto")
                    
                    # Configurar feed para Quote
                    feed_setup = {
                        "type": "FEED_SETUP",
                        "channel": 1,
                        "acceptDataFormat": "COMPACT",
                        "acceptEventFields": {
                            "Quote": ["eventSymbol", "eventTime", "sequence", "timeNanoPart",
                                     "bidTime", "bidExchangeCode", "bidPrice", "bidSize",
                                     "askTime", "askExchangeCode", "askPrice", "askSize"]
                        }
                    }
                    ws.send(json.dumps(feed_setup))
                    
                elif msg_type == "FEED_CONFIG":
                    print(f"{Fore.GREEN}✓ Feed configurado")
                    print(f"{Fore.CYAN}📡 Streaming Quote: {symbol}\n")
                    
                    # Suscribirse a quotes en tiempo real (SIN fromTime)
                    subscribe_msg = {
                        "type": "FEED_SUBSCRIPTION",
                        "channel": 1,
                        "add": [{"type": "Quote", "symbol": symbol}]
                    }
                    ws.send(json.dumps(subscribe_msg))
                    
                elif msg_type == "FEED_DATA":
                    feed_data = data.get("data", [])
                    if feed_data and feed_data[0] == "Quote":
                        raw_data = feed_data[1:] if len(feed_data) > 1 else []
                        
                        for item in raw_data:
                            if isinstance(item, list) and len(item) >= 12:
                                try:
                                    # Formato COMPACT: [eventSymbol, eventTime, sequence, timeNanoPart,
                                    #                   bidTime, bidExchangeCode, bidPrice, bidSize,
                                    #                   askTime, askExchangeCode, askPrice, askSize]
                                    bid_price = float(item[6]) if len(item) > 6 and item[6] else 0.0
                                    ask_price = float(item[10]) if len(item) > 10 and item[10] else 0.0
                                    
                                    if bid_price > 0 and ask_price > 0:
                                        quote = Quote(
                                            timestamp=datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3],
                                            symbol=symbol,
                                            bid_price=bid_price,
                                            bid_size=int(item[7]) if len(item) > 7 and item[7] else 0,
                                            bid_exchange=str(item[5]) if len(item) > 5 and item[5] else "N/A",
                                            ask_price=ask_price,
                                            ask_size=int(item[11]) if len(item) > 11 and item[11] else 0,
                                            ask_exchange=str(item[9]) if len(item) > 9 and item[9] else "N/A",
                                            spread=ask_price - bid_price
                                        )
                                        quotes.append(quote)
                                        
                                        print(f"{Fore.GREEN}📊 #{len(quotes):3d} | "
                                              f"Bid: ${quote.bid_price:.2f} x {quote.bid_size} ({quote.bid_exchange}) | "
                                              f"Ask: ${quote.ask_price:.2f} x {quote.ask_size} ({quote.ask_exchange}) | "
                                              f"Spread: ${quote.spread:.4f}")
                                
                                except (ValueError, TypeError, IndexError) as e:
                                    logger.error(f"Error parseando quote: {e}")
                        
                        if time.time() - start_time >= duration_seconds:
                            print(f"\n{Fore.YELLOW}⏰ Completado")
                            ws.close()
            
            except Exception as e:
                logger.error(f"Error: {e}")
        
        def on_error(ws, error):
            logger.error(f"Error: {error}")
        
        def on_close(ws, close_status_code, close_msg):
            pass
        
        def on_open(ws):
            setup_msg = {
                "type": "SETUP",
                "channel": 0,
                "version": "0.1-DXF-JS/0.3.0",
                "keepaliveTimeout": 60,
                "acceptKeepaliveTimeout": 60
            }
            ws.send(json.dumps(setup_msg))
        
        ws = ws_module.WebSocketApp(
            self.dxlink_url,
            on_open=on_open,
            on_message=on_message,
            on_error=on_error,
            on_close=on_close
        )
        
        import threading
        ws_thread = threading.Thread(target=ws.run_forever)
        ws_thread.daemon = True
        ws_thread.start()
        ws_thread.join(timeout=duration_seconds + 10)
        ws.close()
        
        # Guardar
        if quotes:
            filename = f"realtime_{symbol}_{duration_seconds}s.txt"
            with open(filename, 'w', encoding='utf-8') as f:
                f.write(f"STREAMING EN TIEMPO REAL - {symbol}\n")
                f.write(f"Duración: {duration_seconds}s\n")
                f.write(f"Total: {len(quotes)} quotes\n")
                f.write(f"Fecha: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
                f.write("="*140 + "\n\n")
                f.write(f"{'Timestamp':<26} {'Bid Price':>12} {'Bid Size':>10} {'Bid Exch':>10} "
                       f"{'Ask Price':>12} {'Ask Size':>10} {'Ask Exch':>10} {'Spread':>12}\n")
                f.write("-"*140 + "\n")
                
                for quote in quotes:
                    f.write(f"{quote.timestamp:<26} "
                           f"${quote.bid_price:>11.2f} "
                           f"{quote.bid_size:>10,} "
                           f"{quote.bid_exchange:>10} "
                           f"${quote.ask_price:>11.2f} "
                           f"{quote.ask_size:>10,} "
                           f"{quote.ask_exchange:>10} "
                           f"${quote.spread:>11.4f}\n")
            
            logger.info(f"Guardado en {filename}")
            print(f"\n{Fore.GREEN}✅ Guardado: {filename}")
            
            # Estadísticas
            if len(quotes) > 0:
                avg_spread = sum(q.spread for q in quotes) / len(quotes)
                min_bid = min(q.bid_price for q in quotes)
                max_ask = max(q.ask_price for q in quotes)
                print(f"{Fore.WHITE}   Estadísticas:")
                print(f"{Fore.CYAN}   - Spread promedio: ${avg_spread:.4f}")
                print(f"{Fore.CYAN}   - Bid mínimo: ${min_bid:.2f}")
                print(f"{Fore.CYAN}   - Ask máximo: ${max_ask:.2f}\n")
        else:
            print(f"\n{Fore.YELLOW}⚠️  No se recibieron quotes\n")
            logger.warning("No se recibieron quotes")
        
        return quotes

# ============================================================
# MENÚ PRINCIPAL
# ============================================================

def mostrar_menu():
    """Mostrar menú de opciones"""
    print(f"\n{Fore.YELLOW}{'='*60}")
    print(f"{Fore.YELLOW}MENÚ DE PRUEBAS")
    print(f"{Fore.YELLOW}{'='*60}\n")
    print(f"{Fore.WHITE}1. Test completo (todas las funciones)")
    print(f"{Fore.WHITE}2. Solo obtener símbolos")
    print(f"{Fore.WHITE}3. Solo datos históricos OHLC")
    print(f"{Fore.WHITE}4. Solo streaming en tiempo real (Quote)")
    print(f"{Fore.WHITE}5. Históricos + Streaming (saltar símbolos)")
    print(f"{Fore.WHITE}0. Salir")
    print(f"{Fore.YELLOW}{'='*60}\n")

def main():
    print(f"\n{Fore.CYAN}{'='*60}")
    print(f"{Fore.CYAN}TASTYTRADE API - VERSIÓN CORREGIDA V2")
    print(f"{Fore.CYAN}{'='*60}\n")
    
    # Intentar cargar config
    config = load_config()
    
    if config:
        print(f"{Fore.WHITE}¿Usar credenciales guardadas? (s/n): ", end='')
        usar_guardadas = input().strip().lower()
        
        if usar_guardadas == 's':
            client_id = config["client_id"]
            client_secret = config["client_secret"]
            refresh_token = config["refresh_token"]
            use_production = config["use_production"]
            print(f"{Fore.GREEN}✓ Usando credenciales guardadas\n")
        else:
            config = None
    
    if not config:
        print(f"{Fore.YELLOW}Ingresa credenciales:\n")
        client_id = input(f"{Fore.WHITE}Client ID: ").strip()
        client_secret = input(f"{Fore.WHITE}Client Secret: ").strip()
        refresh_token = input(f"{Fore.WHITE}Refresh Token: ").strip()
        
        print(f"\n{Fore.WHITE}Entorno:")
        print(f"  1. Sandbox")
        print(f"  2. Production")
        env_choice = input(f"{Fore.YELLOW}Selecciona (1/2): ").strip()
        use_production = env_choice == "2"
        
        # Guardar
        print(f"\n{Fore.WHITE}¿Guardar credenciales? (s/n): ", end='')
        guardar = input().strip().lower()
        if guardar == 's':
            save_config(client_id, client_secret, refresh_token, use_production)
    
    # Crear cliente
    client = TastytradeClient(client_id, client_secret, refresh_token, use_production)
    
    # Autenticar
    if not client.authenticate():
        return
    
    if not client.get_dxlink_token():
        return
    
    # Mostrar menú
    while True:
        mostrar_menu()
        opcion = input(f"{Fore.YELLOW}Selecciona una opción: ").strip()
        
        if opcion == "0":
            print(f"\n{Fore.CYAN}👋 Hasta luego!\n")
            break
        
        elif opcion == "1":
            # Test completo
            symbols = client.get_all_symbols()
            
            if symbols:
                symbol_hist = input(f"\n{Fore.YELLOW}Símbolo para históricos: ").strip().upper()
                minutes = input(f"{Fore.YELLOW}Minutos hacia atrás (default 15): ").strip()
                minutes = int(minutes) if minutes else 15
                interval = input(f"{Fore.YELLOW}Intervalo (1m/5m/15m/1h/1d, default 1m): ").strip() or "1m"
                
                historical = client.get_historical_candles(symbol_hist, minutes=minutes, interval=interval)
                
                symbol_stream = input(f"\n{Fore.YELLOW}Símbolo para streaming: ").strip().upper()
                duration = input(f"{Fore.YELLOW}Duración en segundos (default 30): ").strip()
                duration = int(duration) if duration else 30
                
                streaming = client.get_realtime_quotes(symbol_stream, duration_seconds=duration)
                
                # Resumen
                print(f"\n{Fore.CYAN}{'='*60}")
                print(f"{Fore.CYAN}✅ COMPLETADO")
                print(f"{Fore.CYAN}{'='*60}\n")
                print(f"{Fore.GREEN}✓ Símbolos: {len(symbols):,}")
                print(f"{Fore.GREEN}✓ Históricos: {len(historical)} velas")
                print(f"{Fore.GREEN}✓ Streaming: {len(streaming)} quotes")
                print(f"\n{Fore.WHITE}Log guardado en: tastytrade_debug.log\n")
        
        elif opcion == "2":
            # Solo símbolos
            symbols = client.get_all_symbols()
            print(f"{Fore.GREEN}✓ Obtenidos {len(symbols):,} símbolos\n")
        
        elif opcion == "3":
            # Solo históricos
            symbol = input(f"\n{Fore.YELLOW}Símbolo: ").strip().upper()
            minutes = input(f"{Fore.YELLOW}Minutos hacia atrás (default 15): ").strip()
            minutes = int(minutes) if minutes else 15
            interval = input(f"{Fore.YELLOW}Intervalo (1m/5m/15m/1h/1d, default 1m): ").strip() or "1m"
            
            historical = client.get_historical_candles(symbol, minutes=minutes, interval=interval)
            print(f"{Fore.GREEN}✓ Obtenidas {len(historical)} velas\n")
        
        elif opcion == "4":
            # Solo streaming
            symbol = input(f"\n{Fore.YELLOW}Símbolo: ").strip().upper()
            duration = input(f"{Fore.YELLOW}Duración en segundos (default 30): ").strip()
            duration = int(duration) if duration else 30
            
            streaming = client.get_realtime_quotes(symbol, duration_seconds=duration)
            print(f"{Fore.GREEN}✓ Obtenidos {len(streaming)} quotes\n")
        
        elif opcion == "5":
            # Históricos + Streaming (saltar símbolos)
            symbol_hist = input(f"\n{Fore.YELLOW}Símbolo para históricos: ").strip().upper()
            minutes = input(f"{Fore.YELLOW}Minutos hacia atrás (default 15): ").strip()
            minutes = int(minutes) if minutes else 15
            interval = input(f"{Fore.YELLOW}Intervalo (1m/5m/15m/1h/1d, default 1m): ").strip() or "1m"
            
            historical = client.get_historical_candles(symbol_hist, minutes=minutes, interval=interval)
            
            symbol_stream = input(f"\n{Fore.YELLOW}Símbolo para streaming: ").strip().upper()
            duration = input(f"{Fore.YELLOW}Duración en segundos (default 30): ").strip()
            duration = int(duration) if duration else 30
            
            streaming = client.get_realtime_quotes(symbol_stream, duration_seconds=duration)
            
            print(f"\n{Fore.GREEN}✓ Históricos: {len(historical)} velas")
            print(f"{Fore.GREEN}✓ Streaming: {len(streaming)} quotes\n")
        
        else:
            print(f"{Fore.RED}❌ Opción inválida\n")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        logger.info("Interrumpido por usuario")
        print(f"\n\n{Fore.YELLOW}👋 Interrumpido\n")
    except Exception as e:
        logger.exception("Error fatal")
        print(f"\n{Fore.RED}❌ Error: {e}\n")