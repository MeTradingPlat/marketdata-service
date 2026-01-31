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
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.BiConsumer;
import java.util.function.Supplier;

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
    private final ScheduledExecutorService scheduler = Executors.newScheduledThreadPool(2);

    private WebSocketSession session;
    private String apiQuoteToken;
    private String dxLinkUrl;
    private int channelId = 1;

    // Estados de conexión
    private volatile boolean authenticated = false;
    private volatile boolean channelReady = false;
    private volatile boolean feedConfigured = false;

    // Estado del snapshot de candles históricos
    private volatile boolean candleSnapshotComplete = false;
    private volatile boolean candleSubscriptionActive = false;
    private final AtomicInteger candleSnapshotCount = new AtomicInteger(0);

    // Auto-reconexión
    private final AtomicBoolean reconnecting = new AtomicBoolean(false);
    private final AtomicInteger reconnectAttempts = new AtomicInteger(0);
    private static final int MAX_RECONNECT_ATTEMPTS = 10;
    private static final int INITIAL_RECONNECT_DELAY_SECONDS = 5;
    private static final int MAX_RECONNECT_DELAY_SECONDS = 300; // 5 minutos máximo
    private static final int HEALTH_CHECK_INTERVAL_SECONDS = 60;

    private ScheduledFuture<?> keepaliveTask;
    private ScheduledFuture<?> healthCheckTask;
    private Supplier<String> tokenRefresher;

    private BiConsumer<String, MarketDataStreamDTO> onMarketData;
    private BiConsumer<String, Candle> onCandle;

    public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> callback) {
        this.onMarketData = callback;
    }

    public void setOnCandle(BiConsumer<String, Candle> callback) {
        this.onCandle = callback;
    }

    /**
     * Configura el proveedor de tokens frescos para auto-reconexión.
     * Este supplier será llamado cada vez que se necesite reconectar.
     */
    public void setTokenRefresher(Supplier<String> tokenRefresher) {
        this.tokenRefresher = tokenRefresher;
    }

    /**
     * Conecta al WebSocket DxLink y espera que esté completamente listo.
     */
    public void connect(String url, String token) {
        this.dxLinkUrl = url;
        this.apiQuoteToken = token;
        this.authenticated = false;
        this.channelReady = false;
        this.feedConfigured = false;
        this.reconnectAttempts.set(0);

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
                // Iniciar health check periódico
                startHealthCheck();
            } else {
                log.warn("DxLink connection timeout after 30s - auth={}, channel={}, feed={}",
                    authenticated, channelReady, feedConfigured);
                // Intentar cerrar la conexión y reportar el estado
                if (session != null && session.isOpen()) {
                    log.info("Session is still open but no SETUP received. Remote address: {}",
                        session.getRemoteAddress());
                }
                // Programar reconexión
                scheduleReconnect();
            }
        } catch (Exception e) {
            log.error("Failed to connect to DxLink: {}", e.getMessage(), e);
            // Programar reconexión en lugar de lanzar excepción
            scheduleReconnect();
        }
    }

    /**
     * Intenta reconectar al WebSocket con backoff exponencial.
     */
    private void scheduleReconnect() {
        if (!reconnecting.compareAndSet(false, true)) {
            log.debug("Reconnection already in progress, skipping");
            return;
        }

        int attempts = reconnectAttempts.incrementAndGet();
        if (attempts > MAX_RECONNECT_ATTEMPTS) {
            log.error("Max reconnection attempts ({}) reached. Manual intervention required.", MAX_RECONNECT_ATTEMPTS);
            reconnecting.set(false);
            reconnectAttempts.set(0);
            return;
        }

        // Backoff exponencial: 5s, 10s, 20s, 40s, 80s, 160s, 300s (max)
        int delaySeconds = Math.min(
            INITIAL_RECONNECT_DELAY_SECONDS * (int) Math.pow(2, attempts - 1),
            MAX_RECONNECT_DELAY_SECONDS
        );

        log.info("Scheduling reconnection attempt {}/{} in {} seconds...",
            attempts, MAX_RECONNECT_ATTEMPTS, delaySeconds);

        scheduler.schedule(() -> {
            try {
                performReconnect();
            } finally {
                reconnecting.set(false);
            }
        }, delaySeconds, TimeUnit.SECONDS);
    }

    /**
     * Ejecuta la reconexión real.
     */
    private void performReconnect() {
        log.info("Attempting to reconnect to DxLink...");

        // Limpiar estado anterior
        cleanupConnection();

        // Obtener token fresco si hay un refresher configurado
        String freshToken = apiQuoteToken;
        if (tokenRefresher != null) {
            try {
                freshToken = tokenRefresher.get();
                log.info("Obtained fresh token for reconnection");
            } catch (Exception e) {
                log.error("Failed to refresh token: {}", e.getMessage());
                scheduleReconnect();
                return;
            }
        }

        // Intentar reconectar
        try {
            connect(dxLinkUrl, freshToken);

            if (channelReady) {
                log.info("Reconnection successful! Re-subscribing to {} symbols...", subscribedSymbols.size());
                reconnectAttempts.set(0);

                // Re-suscribir a todos los símbolos anteriores
                resubscribeAll();
            }
        } catch (Exception e) {
            log.error("Reconnection failed: {}", e.getMessage());
            scheduleReconnect();
        }
    }

    /**
     * Re-suscribe a todos los símbolos que estaban activos antes de la desconexión.
     */
    private void resubscribeAll() {
        if (subscribedSymbols.isEmpty()) {
            return;
        }

        log.info("Re-subscribing to {} symbols: {}", subscribedSymbols.size(), subscribedSymbols);

        // Crear copia para evitar ConcurrentModificationException
        Set<String> symbolsToResubscribe = Set.copyOf(subscribedSymbols);

        for (String symbol : symbolsToResubscribe) {
            try {
                sendMessage(Map.of(
                    "type", "FEED_SUBSCRIPTION",
                    "channel", channelId,
                    "add", List.of(
                        Map.of("symbol", symbol, "type", "Quote"),
                        Map.of("symbol", symbol, "type", "Trade")
                    )
                ));
                log.debug("Re-subscribed to: {}", symbol);
            } catch (Exception e) {
                log.error("Failed to re-subscribe to {}: {}", symbol, e.getMessage());
            }
        }

        log.info("Re-subscription complete");
    }

    /**
     * Limpia la conexión anterior antes de reconectar.
     */
    private void cleanupConnection() {
        authenticated = false;
        channelReady = false;
        feedConfigured = false;

        if (keepaliveTask != null) {
            keepaliveTask.cancel(false);
            keepaliveTask = null;
        }

        if (session != null && session.isOpen()) {
            try {
                session.close();
            } catch (Exception e) {
                log.debug("Error closing old session: {}", e.getMessage());
            }
        }
        session = null;
    }

    /**
     * Inicia el health check periódico de la conexión.
     */
    private void startHealthCheck() {
        if (healthCheckTask != null) {
            healthCheckTask.cancel(false);
        }

        healthCheckTask = scheduler.scheduleAtFixedRate(() -> {
            try {
                checkConnectionHealth();
            } catch (Exception e) {
                log.error("Health check error: {}", e.getMessage());
            }
        }, HEALTH_CHECK_INTERVAL_SECONDS, HEALTH_CHECK_INTERVAL_SECONDS, TimeUnit.SECONDS);

        log.info("Health check started (interval: {}s)", HEALTH_CHECK_INTERVAL_SECONDS);
    }

    /**
     * Verifica el estado de la conexión y reconecta si es necesario.
     */
    private void checkConnectionHealth() {
        boolean sessionOpen = session != null && session.isOpen();
        boolean healthy = sessionOpen && authenticated && channelReady;

        if (!healthy) {
            log.warn("Connection health check failed - session={}, auth={}, channel={}. Triggering reconnect...",
                sessionOpen, authenticated, channelReady);
            scheduleReconnect();
        } else {
            log.debug("Connection health check passed");
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

        // Resetear estado del snapshot
        candleSnapshotComplete = false;
        candleSubscriptionActive = true;
        candleSnapshotCount.set(0);

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
     * Desuscribe de candles de un símbolo.
     */
    public void unsubscribeCandles(String symbol, String timeframe) {
        candleSubscriptionActive = false;
        String candleSymbol = symbol + "{=" + timeframe + "}";
        sendMessage(Map.of(
            "type", "FEED_SUBSCRIPTION",
            "channel", channelId,
            "remove", List.of(Map.of(
                "symbol", candleSymbol,
                "type", "Candle"
            ))
        ));
        log.info("Unsubscribed from candles: {}", candleSymbol);
    }

    /**
     * Verifica si el snapshot de candles está completo.
     */
    public boolean isCandleSnapshotComplete() {
        return candleSnapshotComplete;
    }

    /**
     * Obtiene el número de candles recibidos en el snapshot actual.
     */
    public int getCandleSnapshotCount() {
        return candleSnapshotCount.get();
    }

    /**
     * Resetea el estado del snapshot de candles.
     */
    public void resetCandleSnapshot() {
        candleSnapshotComplete = false;
        candleSnapshotCount.set(0);
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
        log.info("Disconnecting from DxLink...");
        try {
            // Cancelar tareas programadas
            if (healthCheckTask != null) {
                healthCheckTask.cancel(true);
                healthCheckTask = null;
            }
            if (keepaliveTask != null) {
                keepaliveTask.cancel(true);
                keepaliveTask = null;
            }
            // Cerrar sesión con código NORMAL para evitar auto-reconexión
            if (session != null && session.isOpen()) {
                session.close(CloseStatus.NORMAL);
            }
            authenticated = false;
            channelReady = false;
            feedConfigured = false;
        } catch (Exception e) {
            log.error("Error closing WebSocket", e);
        }
    }

    public boolean isConnected() {
        return session != null && session.isOpen() && channelReady;
    }

    /**
     * Fuerza una reconexión inmediata (útil para recuperación manual).
     */
    public void forceReconnect() {
        log.info("Forcing reconnection...");
        reconnectAttempts.set(0);
        reconnecting.set(false);
        cleanupConnection();
        scheduleReconnect();
    }

    /**
     * Obtiene estadísticas de conexión para monitoreo.
     */
    public Map<String, Object> getConnectionStats() {
        return Map.of(
            "connected", isConnected(),
            "authenticated", authenticated,
            "channelOpened", channelReady,
            "feedConfigured", feedConfigured,
            "activeSubscriptions", subscribedSymbols.size(),
            "subscribedSymbolsList", List.copyOf(subscribedSymbols),
            "reconnectAttempts", reconnectAttempts.get(),
            "reconnecting", reconnecting.get()
        );
    }

    @PreDestroy
    public void cleanup() {
        log.info("Cleaning up DxLinkClient...");
        disconnect();
        scheduler.shutdown();
        try {
            if (!scheduler.awaitTermination(5, TimeUnit.SECONDS)) {
                scheduler.shutdownNow();
            }
        } catch (InterruptedException e) {
            scheduler.shutdownNow();
            Thread.currentThread().interrupt();
        }
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
            // Configurar el feed - incluir eventFlags para detectar fin de snapshot
            sendMessage(Map.of(
                "type", "FEED_SETUP",
                "channel", channelId,
                "acceptDataFormat", "COMPACT",
                "acceptEventFields", Map.of(
                    "Quote", List.of("eventSymbol", "bidPrice", "askPrice", "bidSize", "askSize"),
                    "Trade", List.of("eventSymbol", "price", "size", "time"),
                    "Candle", List.of("eventSymbol", "time", "open", "high", "low", "close", "volume", "eventFlags")
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
                    double bid = data.path(1).asDouble();
                    double ask = data.path(2).asDouble();
                    log.info("Quote received for {}: bid={}, ask={}", symbol, bid, ask);
                    if (onMarketData != null) {
                        MarketDataStreamDTO dto = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .bid(bid)
                            .ask(ask)
                            .timestamp(Instant.now())
                            .build();
                        onMarketData.accept(symbol, dto);
                    } else {
                        log.warn("Quote received but onMarketData callback is null!");
                    }
                }
                case "Trade" -> {
                    double price = data.path(1).asDouble();
                    long volume = data.path(2).asLong();
                    long time = data.path(3).asLong();
                    log.info("Trade received for {}: price={}, volume={}, time={}", symbol, price, volume, time);
                    if (onMarketData != null) {
                        MarketDataStreamDTO dto = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .lastPrice(price)
                            .volume(volume)
                            .timestamp(Instant.ofEpochMilli(time))
                            .build();
                        onMarketData.accept(symbol, dto);
                    } else {
                        log.warn("Trade received but onMarketData callback is null!");
                    }
                }
                case "Candle" -> {
                    if (onCandle != null && candleSubscriptionActive) {
                        // Formato COMPACT: los datos vienen como array plano con múltiples candles
                        // Cada candle tiene 8 campos: symbol, time, open, high, low, close, volume, eventFlags
                        int fieldsPerCandle = 8;
                        int totalCandles = data.size() / fieldsPerCandle;

                        log.info("Processing {} candles from COMPACT array (size={})", totalCandles, data.size());

                        boolean lastCandleTxPending = false;

                        for (int idx = 0; idx < data.size(); idx += fieldsPerCandle) {
                            String candleSymbol = data.path(idx).asText();
                            String baseSymbol = candleSymbol.contains("{")
                                ? candleSymbol.substring(0, candleSymbol.indexOf("{"))
                                : candleSymbol;

                            long timestamp = data.path(idx + 1).asLong();
                            double open = data.path(idx + 2).asDouble();
                            double high = data.path(idx + 3).asDouble();
                            double low = data.path(idx + 4).asDouble();
                            double close = data.path(idx + 5).asDouble();
                            double volume = data.path(idx + 6).asDouble();
                            int eventFlags = data.path(idx + 7).asInt(0);

                            // Skip candles with NaN values (incomplete data)
                            if (Double.isNaN(open) || Double.isNaN(high) || Double.isNaN(low) || Double.isNaN(close)) {
                                log.debug("Skipping candle with NaN values at index {}", idx);
                                continue;
                            }

                            boolean isTxPending = (eventFlags & 0x01) != 0;
                            lastCandleTxPending = isTxPending;

                            Candle candle = Candle.builder()
                                .symbol(baseSymbol)
                                .timestamp(Instant.ofEpochMilli(timestamp))
                                .open(open)
                                .high(high)
                                .low(low)
                                .close(close)
                                .volume(volume)
                                .build();

                            candleSnapshotCount.incrementAndGet();

                            log.info("Received candle #{}: {} at {} O={} H={} L={} C={} V={} flags={} txPending={}",
                                candleSnapshotCount.get(), baseSymbol, candle.getTimestamp(),
                                candle.getOpen(), candle.getHigh(), candle.getLow(), candle.getClose(),
                                candle.getVolume(), eventFlags, isTxPending);

                            onCandle.accept(baseSymbol, candle);
                        }

                        // Si el último candle no tiene TX_PENDING, el snapshot terminó
                        if (!lastCandleTxPending && candleSnapshotCount.get() > 0) {
                            candleSnapshotComplete = true;
                            log.info("Candle snapshot complete! Total candles: {}", candleSnapshotCount.get());
                        }
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
                    if (onCandle != null && candleSubscriptionActive) {
                        String baseSymbol = symbol.contains("{")
                            ? symbol.substring(0, symbol.indexOf("{"))
                            : symbol;

                        // eventFlags para detectar fin de snapshot
                        int eventFlags = data.path("eventFlags").asInt(0);
                        boolean isTxPending = (eventFlags & 0x01) != 0;

                        Candle candle = Candle.builder()
                            .symbol(baseSymbol)
                            .timestamp(Instant.ofEpochMilli(data.path("time").asLong()))
                            .open(data.path("open").asDouble())
                            .high(data.path("high").asDouble())
                            .low(data.path("low").asDouble())
                            .close(data.path("close").asDouble())
                            .volume(data.path("volume").asDouble())
                            .build();

                        candleSnapshotCount.incrementAndGet();

                        log.info("Received candle #{}: {} at {} O={} H={} L={} C={} flags={} txPending={}",
                            candleSnapshotCount.get(), baseSymbol, candle.getTimestamp(),
                            candle.getOpen(), candle.getHigh(), candle.getLow(), candle.getClose(),
                            eventFlags, isTxPending);

                        onCandle.accept(baseSymbol, candle);

                        // Si TX_PENDING no está presente, el snapshot terminó
                        if (!isTxPending && candleSnapshotCount.get() > 0) {
                            candleSnapshotComplete = true;
                            log.info("Candle snapshot complete! Total candles: {}", candleSnapshotCount.get());
                        }
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

            // Disparar reconexión automática (excepto si fue un cierre intencional)
            if (status.getCode() != CloseStatus.NORMAL.getCode()) {
                log.info("Connection lost unexpectedly. Initiating auto-reconnect...");
                scheduleReconnect();
            }
        }
    }
}
