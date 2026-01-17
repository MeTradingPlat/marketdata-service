package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.function.BiConsumer;

import org.springframework.stereotype.Component;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketHttpHeaders;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.TextWebSocketHandler;

import jakarta.websocket.ContainerProvider;
import jakarta.websocket.WebSocketContainer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.MarketDataStreamDTO;

import jakarta.annotation.PreDestroy;
import lombok.extern.slf4j.Slf4j;

/**
 * Cliente WebSocket para DxLink (streaming de datos de mercado).
 * Implementa el protocolo dxLink WebSocket 1.0.2
 *
 * Flujo del protocolo:
 * 1. Conectar WebSocket
 * 2. Recibir SETUP del servidor
 * 3. Enviar SETUP al servidor
 * 4. Enviar AUTH con token
 * 5. Recibir AUTH_STATE = AUTHORIZED
 * 6. Enviar CHANNEL_REQUEST para FEED
 * 7. Recibir CHANNEL_OPENED
 * 8. Enviar FEED_SETUP (configuración)
 * 9. Recibir FEED_CONFIG
 * 10. Enviar FEED_SUBSCRIPTION
 * 11. Recibir FEED_DATA
 */
@Component
@Slf4j
public class DxLinkClient {

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final Set<String> subscribedSymbols = ConcurrentHashMap.newKeySet();
    private final ScheduledExecutorService scheduler = Executors.newScheduledThreadPool(1);

    private WebSocketSession session;
    private String apiQuoteToken;
    private int channelId = 1;

    // Estados de conexión
    private volatile boolean authenticated = false;
    private volatile boolean channelReady = false;
    private volatile boolean feedConfigured = false;

    private ScheduledFuture<?> keepaliveTask;

    private BiConsumer<String, MarketDataStreamDTO> onMarketData;
    private BiConsumer<String, Candle> onCandle;

    public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> callback) {
        this.onMarketData = callback;
    }

    public void setOnCandle(BiConsumer<String, Candle> callback) {
        this.onCandle = callback;
    }

    /**
     * Conecta al WebSocket DxLink y espera que esté completamente listo.
     */
    public void connect(String url, String token) {
        this.apiQuoteToken = token;
        this.authenticated = false;
        this.channelReady = false;
        this.feedConfigured = false;

        log.info("Connecting to DxLink: {}", url);
        log.info("Token prefix: {}", token != null ? token.substring(0, Math.min(10, token.length())) + "..." : "NULL");

        try {
            // Configurar WebSocket container con timeouts extendidos
            WebSocketContainer container = ContainerProvider.getWebSocketContainer();
            container.setDefaultMaxSessionIdleTimeout(120000); // 2 minutos
            container.setDefaultMaxTextMessageBufferSize(65536);

            StandardWebSocketClient client = new StandardWebSocketClient(container);

            // Headers HTTP para la conexión WebSocket
            WebSocketHttpHeaders headers = new WebSocketHttpHeaders();
            headers.add("User-Agent", "metradingplat/1.0");

            log.info("Initiating WebSocket connection...");
            // La sesión se guarda en afterConnectionEstablished del handler
            client.execute(new DxLinkHandler(), headers, java.net.URI.create(url)).get(30, TimeUnit.SECONDS);
            log.info("WebSocket connection initiated, waiting for handshake to complete...");

            // Esperar hasta que el canal esté listo (máximo 30 segundos)
            int waitCount = 0;
            while (!channelReady && waitCount < 300) {
                Thread.sleep(100);
                waitCount++;
                // Log progress cada 5 segundos
                if (waitCount % 50 == 0) {
                    log.info("Waiting for handshake... auth={}, channel={}, feed={} ({}s elapsed)",
                        authenticated, channelReady, feedConfigured, waitCount / 10);
                }
            }

            if (channelReady) {
                log.info("DxLink fully connected and ready (auth={}, channel={}, feed={})",
                    authenticated, channelReady, feedConfigured);
            } else {
                log.warn("DxLink connection timeout after 30s - auth={}, channel={}, feed={}",
                    authenticated, channelReady, feedConfigured);
                // Intentar cerrar la conexión y reportar el estado
                if (session != null && session.isOpen()) {
                    log.info("Session is still open but no SETUP received. Remote address: {}",
                        session.getRemoteAddress());
                }
            }
        } catch (Exception e) {
            log.error("Failed to connect to DxLink: {}", e.getMessage(), e);
            throw new RuntimeException("DxLink connection failed: " + e.getMessage(), e);
        }
    }

    /**
     * Suscribe a quotes y trades de un símbolo.
     */
    public void subscribe(String symbol) {
        if (!waitForReady()) {
            log.warn("Cannot subscribe - channel not ready. Queuing: {}", symbol);
            subscribedSymbols.add(symbol);
            return;
        }

        if (subscribedSymbols.contains(symbol)) {
            log.debug("Already subscribed to: {}", symbol);
            return;
        }

        subscribedSymbols.add(symbol);

        // Formato correcto según documentación
        sendMessage(Map.of(
            "type", "FEED_SUBSCRIPTION",
            "channel", channelId,
            "add", List.of(
                Map.of("symbol", symbol, "type", "Quote"),
                Map.of("symbol", symbol, "type", "Trade")
            )
        ));
        log.info("Subscribed to Quote and Trade for: {}", symbol);
    }

    /**
     * Desuscribe de un símbolo.
     */
    public void unsubscribe(String symbol) {
        if (!subscribedSymbols.remove(symbol)) {
            return;
        }

        sendMessage(Map.of(
            "type", "FEED_SUBSCRIPTION",
            "channel", channelId,
            "remove", List.of(
                Map.of("symbol", symbol, "type", "Quote"),
                Map.of("symbol", symbol, "type", "Trade")
            )
        ));
        log.info("Unsubscribed from: {}", symbol);
    }

    /**
     * Suscribe a candles históricos.
     * El símbolo de candle tiene formato: AAPL{=5m} para 5 minutos
     */
    public void subscribeCandles(String symbol, String timeframe, long fromTime) {
        if (!waitForReady()) {
            log.warn("Cannot subscribe to candles - channel not ready");
            return;
        }

        // Formato del símbolo de candle: SYMBOL{=TIMEFRAME}
        // Timeframes: 1m, 5m, 15m, 30m, 1h, 1d, 1w, 1mo
        String candleSymbol = symbol + "{=" + timeframe + "}";

        // Suscripción de tipo TimeSeries con fromTime
        sendMessage(Map.of(
            "type", "FEED_SUBSCRIPTION",
            "channel", channelId,
            "add", List.of(Map.of(
                "symbol", candleSymbol,
                "type", "Candle",
                "fromTime", fromTime
            ))
        ));
        log.info("Subscribed to candles: {} from {}", candleSymbol, Instant.ofEpochMilli(fromTime));
    }

    /**
     * Espera hasta que el canal esté listo (máximo 10 segundos)
     */
    private boolean waitForReady() {
        int waitCount = 0;
        while (!channelReady && waitCount < 100) {
            try {
                Thread.sleep(100);
                waitCount++;
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            }
        }
        return channelReady;
    }

    public void disconnect() {
        try {
            if (keepaliveTask != null) {
                keepaliveTask.cancel(true);
            }
            if (session != null && session.isOpen()) {
                session.close();
            }
        } catch (Exception e) {
            log.error("Error closing WebSocket", e);
        }
    }

    public boolean isConnected() {
        return session != null && session.isOpen() && channelReady;
    }

    @PreDestroy
    public void cleanup() {
        disconnect();
        scheduler.shutdown();
    }

    private void sendMessage(Object message) {
        try {
            if (session == null || !session.isOpen()) {
                log.warn("Cannot send message - session not open");
                return;
            }
            String json = objectMapper.writeValueAsString(message);
            log.info(">>> Sending: {}", json);
            session.sendMessage(new TextMessage(json));
        } catch (Exception e) {
            log.error("Failed to send message", e);
        }
    }

    private void handleMessage(String payload) {
        try {
            log.debug("Received: {}", payload);
            JsonNode msg = objectMapper.readTree(payload);
            String type = msg.path("type").asText();

            switch (type) {
                case "SETUP" -> handleSetup(msg);
                case "AUTH_STATE" -> handleAuthState(msg);
                case "CHANNEL_OPENED" -> handleChannelOpened(msg);
                case "FEED_CONFIG" -> handleFeedConfig(msg);
                case "FEED_DATA" -> handleFeedData(msg);
                case "KEEPALIVE" -> {
                    // Responder al keepalive del servidor
                    sendMessage(Map.of("type", "KEEPALIVE", "channel", 0));
                }
                case "ERROR" -> {
                    log.error("DxLink error: {}", msg.path("error").asText());
                }
                default -> log.debug("Unhandled message type: {}", type);
            }
        } catch (Exception e) {
            log.error("Error handling message: {}", payload, e);
        }
    }

    /**
     * El servidor responde con SETUP después de recibir nuestro SETUP.
     * Ahora enviamos AUTH con el token.
     */
    private void handleSetup(JsonNode msg) {
        log.info("Received SETUP from server: version={}, keepaliveTimeout={}",
            msg.path("version").asText(), msg.path("keepaliveTimeout").asInt());

        // Enviar AUTH con el token
        log.info("Sending AUTH with token...");
        sendMessage(Map.of(
            "type", "AUTH",
            "channel", 0,
            "token", apiQuoteToken
        ));
    }

    private void handleAuthState(JsonNode msg) {
        String state = msg.path("state").asText();
        log.info("AUTH_STATE received: {}", state);

        if ("AUTHORIZED".equals(state)) {
            authenticated = true;
            log.info("DxLink authenticated successfully");

            // Solicitar canal FEED
            sendMessage(Map.of(
                "type", "CHANNEL_REQUEST",
                "channel", channelId,
                "service", "FEED",
                "parameters", Map.of("contract", "AUTO")
            ));
        } else {
            log.error("Authentication failed: {}", state);
            authenticated = false;
        }
    }

    private void handleChannelOpened(JsonNode msg) {
        int openedChannel = msg.path("channel").asInt();
        String service = msg.path("service").asText();
        log.info("Channel {} opened for service: {}", openedChannel, service);

        if (openedChannel == channelId && "FEED".equals(service)) {
            // Configurar el feed
            sendMessage(Map.of(
                "type", "FEED_SETUP",
                "channel", channelId,
                "acceptDataFormat", "COMPACT",
                "acceptEventFields", Map.of(
                    "Quote", List.of("eventSymbol", "bidPrice", "askPrice", "bidSize", "askSize"),
                    "Trade", List.of("eventSymbol", "price", "size", "time"),
                    "Candle", List.of("eventSymbol", "time", "open", "high", "low", "close", "volume")
                )
            ));

            // Marcar canal como listo
            channelReady = true;

            // Iniciar keepalive
            startKeepalive();

            // Suscribir símbolos pendientes
            if (!subscribedSymbols.isEmpty()) {
                log.info("Subscribing to pending symbols: {}", subscribedSymbols);
                for (String symbol : subscribedSymbols) {
                    sendMessage(Map.of(
                        "type", "FEED_SUBSCRIPTION",
                        "channel", channelId,
                        "add", List.of(
                            Map.of("symbol", symbol, "type", "Quote"),
                            Map.of("symbol", symbol, "type", "Trade")
                        )
                    ));
                }
            }
        }
    }

    private void handleFeedConfig(JsonNode msg) {
        log.info("FEED_CONFIG received: dataFormat={}", msg.path("dataFormat").asText());
        feedConfigured = true;
    }

    private void startKeepalive() {
        if (keepaliveTask != null) {
            keepaliveTask.cancel(true);
        }
        keepaliveTask = scheduler.scheduleAtFixedRate(
            () -> {
                if (session != null && session.isOpen()) {
                    sendMessage(Map.of("type", "KEEPALIVE", "channel", 0));
                }
            },
            30, 30, TimeUnit.SECONDS
        );
    }

    private void handleFeedData(JsonNode msg) {
        JsonNode data = msg.path("data");
        if (!data.isArray() || data.isEmpty()) {
            return;
        }

        // En formato COMPACT, los datos vienen como:
        // ["Quote", [eventSymbol, bidPrice, askPrice, bidSize, askSize], "Trade", [...], ...]
        // o pueden venir como objetos si el servidor no respetó COMPACT

        int i = 0;
        while (i < data.size()) {
            JsonNode item = data.get(i);

            if (item.isTextual()) {
                // Formato COMPACT: tipo seguido de array de valores
                String eventType = item.asText();
                if (i + 1 < data.size()) {
                    i++;
                    JsonNode eventData = data.get(i);
                    if (eventData.isArray()) {
                        processCompactEvent(eventType, eventData);
                    }
                }
            } else if (item.isObject()) {
                // Formato FULL: objeto con eventType
                String eventType = item.path("eventType").asText();
                if (eventType.isEmpty()) {
                    eventType = item.path("type").asText();
                }
                processFullEvent(eventType, item);
            }
            i++;
        }
    }

    private void processCompactEvent(String eventType, JsonNode data) {
        try {
            String symbol = data.path(0).asText();
            log.debug("Processing COMPACT {} for {}", eventType, symbol);

            switch (eventType) {
                case "Quote" -> {
                    if (onMarketData != null) {
                        MarketDataStreamDTO dto = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .bid(data.path(1).asDouble())
                            .ask(data.path(2).asDouble())
                            .timestamp(Instant.now())
                            .build();
                        onMarketData.accept(symbol, dto);
                    }
                }
                case "Trade" -> {
                    if (onMarketData != null) {
                        MarketDataStreamDTO dto = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .lastPrice(data.path(1).asDouble())
                            .volume(data.path(2).asLong())
                            .timestamp(Instant.ofEpochMilli(data.path(3).asLong()))
                            .build();
                        onMarketData.accept(symbol, dto);
                    }
                }
                case "Candle" -> {
                    if (onCandle != null) {
                        String baseSymbol = symbol.contains("{")
                            ? symbol.substring(0, symbol.indexOf("{"))
                            : symbol;
                        Candle candle = Candle.builder()
                            .symbol(baseSymbol)
                            .timestamp(Instant.ofEpochMilli(data.path(1).asLong()))
                            .open(data.path(2).asDouble())
                            .high(data.path(3).asDouble())
                            .low(data.path(4).asDouble())
                            .close(data.path(5).asDouble())
                            .volume(data.path(6).asDouble())
                            .build();
                        log.debug("Received candle: {} at {} O={} H={} L={} C={}",
                            baseSymbol, candle.getTimestamp(), candle.getOpen(),
                            candle.getHigh(), candle.getLow(), candle.getClose());
                        onCandle.accept(baseSymbol, candle);
                    }
                }
            }
        } catch (Exception e) {
            log.error("Error processing COMPACT {} event: {}", eventType, data, e);
        }
    }

    private void processFullEvent(String eventType, JsonNode data) {
        try {
            String symbol = data.path("eventSymbol").asText();
            log.debug("Processing FULL {} for {}", eventType, symbol);

            switch (eventType) {
                case "Quote" -> {
                    if (onMarketData != null) {
                        MarketDataStreamDTO dto = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .bid(data.path("bidPrice").asDouble())
                            .ask(data.path("askPrice").asDouble())
                            .timestamp(Instant.now())
                            .build();
                        onMarketData.accept(symbol, dto);
                    }
                }
                case "Trade" -> {
                    if (onMarketData != null) {
                        MarketDataStreamDTO dto = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .lastPrice(data.path("price").asDouble())
                            .volume(data.path("size").asLong())
                            .timestamp(Instant.ofEpochMilli(data.path("time").asLong()))
                            .build();
                        onMarketData.accept(symbol, dto);
                    }
                }
                case "Candle" -> {
                    if (onCandle != null) {
                        String baseSymbol = symbol.contains("{")
                            ? symbol.substring(0, symbol.indexOf("{"))
                            : symbol;
                        Candle candle = Candle.builder()
                            .symbol(baseSymbol)
                            .timestamp(Instant.ofEpochMilli(data.path("time").asLong()))
                            .open(data.path("open").asDouble())
                            .high(data.path("high").asDouble())
                            .low(data.path("low").asDouble())
                            .close(data.path("close").asDouble())
                            .volume(data.path("volume").asDouble())
                            .build();
                        log.debug("Received candle: {} at {} O={} H={} L={} C={}",
                            baseSymbol, candle.getTimestamp(), candle.getOpen(),
                            candle.getHigh(), candle.getLow(), candle.getClose());
                        onCandle.accept(baseSymbol, candle);
                    }
                }
            }
        } catch (Exception e) {
            log.error("Error processing FULL {} event: {}", eventType, data, e);
        }
    }

    private class DxLinkHandler extends TextWebSocketHandler {
        @Override
        public void afterConnectionEstablished(WebSocketSession wsSession) {
            log.info("WebSocket connection established - local: {}, remote: {}, protocol: {}",
                wsSession.getLocalAddress(), wsSession.getRemoteAddress(), wsSession.getAcceptedProtocol());
            log.info("WebSocket attributes: {}", wsSession.getAttributes());

            // Guardar la sesión y enviar SETUP directamente usando la sesión local
            DxLinkClient.this.session = wsSession;

            // El CLIENTE debe enviar SETUP primero según el protocolo dxLink
            log.info("Sending initial SETUP message to server...");
            try {
                String setupJson = objectMapper.writeValueAsString(Map.of(
                    "type", "SETUP",
                    "channel", 0,
                    "version", "0.1-js/1.0.0",
                    "keepaliveTimeout", 60,
                    "acceptKeepaliveTimeout", 60
                ));
                log.info(">>> Sending: {}", setupJson);
                wsSession.sendMessage(new TextMessage(setupJson));
            } catch (Exception e) {
                log.error("Failed to send initial SETUP", e);
            }
        }

        @Override
        protected void handleTextMessage(WebSocketSession session, TextMessage message) {
            String payload = message.getPayload();
            log.info("<<< Received message ({}B): {}", payload.length(),
                payload.length() > 500 ? payload.substring(0, 500) + "..." : payload);
            DxLinkClient.this.handleMessage(payload);
        }

        @Override
        public void handleTransportError(WebSocketSession session, Throwable exception) {
            log.error("WebSocket transport error: {}", exception.getMessage(), exception);
        }

        @Override
        public void afterConnectionClosed(WebSocketSession session, CloseStatus status) {
            log.warn("WebSocket closed - code: {}, reason: '{}'", status.getCode(), status.getReason());
            authenticated = false;
            channelReady = false;
            feedConfigured = false;
        }
    }
}
