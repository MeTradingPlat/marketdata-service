package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
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
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.domain.models.OptionContract;
import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.MarketDataStreamDTO;

import jakarta.annotation.PreDestroy;
import lombok.extern.slf4j.Slf4j;

/**
 * Cliente WebSocket para DxLink (streaming de datos de mercado).
 * Implementa el protocolo dxLink WebSocket 1.0.2 con soporte para múltiples
 * canales (Multiplexing).
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

    // Gestión de Canales
    private final Map<Integer, DxLinkChannel> channels = new ConcurrentHashMap<>();
    // DxLink reserva canales pares para el servidor; el cliente DEBE usar impares (1, 3, 5, ...).
    // Inicializamos en -1 para que el primer addAndGet(2) dé 1 (default channel).
    private final AtomicInteger nextChannelId = new AtomicInteger(-1);
    private DxLinkChannel defaultChannel; // Canal por defecto para streaming continuo

    public DxLinkChannel getDefaultChannel() {
        return defaultChannel;
    }

    // Estados de conexión (Nivel Socket)
    private volatile boolean authenticated = false;

    // Auto-reconexión
    private final AtomicBoolean reconnecting = new AtomicBoolean(false);
    private final AtomicInteger reconnectAttempts = new AtomicInteger(0);
    private static final int MAX_RECONNECT_ATTEMPTS = 10;
    private static final int INITIAL_RECONNECT_DELAY_SECONDS = 5;
    private static final int MAX_RECONNECT_DELAY_SECONDS = 300;
    private static final int HEALTH_CHECK_INTERVAL_SECONDS = 60;

    private ScheduledFuture<?> keepaliveTask;
    private ScheduledFuture<?> healthCheckTask;
    private final List<CandleCallback> candleListeners = new java.util.concurrent.CopyOnWriteArrayList<>();
    private final List<BiConsumer<String, FundamentalData>> fundamentalListeners = new java.util.concurrent.CopyOnWriteArrayList<>();
    private final List<BiConsumer<String, OptionContract>> greeksListeners = new java.util.concurrent.CopyOnWriteArrayList<>();
    private final List<BiConsumer<String, JsonNode>> messageListeners = new java.util.concurrent.CopyOnWriteArrayList<>();
    private Supplier<String> tokenRefresher;

    // --- Definición de Contratos para Formato COMPACT (Punto 1 y 2 del manual) ---
    private static final List<String> QUOTE_FIELDS = List.of("eventSymbol", "bidPrice", "askPrice");
    private static final List<String> TRADE_FIELDS = List.of("eventSymbol", "price", "dayVolume", "time");
    private static final List<String> SUMMARY_FIELDS = List.of("eventSymbol", "dayOpenPrice", "dayHighPrice", "dayLowPrice", "prevDayClosePrice", "openInterest");
    private static final List<String> PROFILE_FIELDS = List.of("eventSymbol", "shares", "earningsPerShare", "exDividendAmount", "dividendFrequency", "tradingStatus", "statusReason", "haltStartTime", "haltEndTime", "beta", "freeFloat");
    private static final List<String> TRADE_ETH_FIELDS = List.of("eventSymbol", "dayVolume", "extendedTradingHours", "time");
    private static final List<String> GREEKS_FIELDS = List.of("eventSymbol", "volatility", "delta", "gamma", "theta", "vega", "rho", "theoreticalPrice");

    // Índices (basados en las listas anteriores, +1 porque el índice 0 es siempre el EventType)
    private static final int IDX_SYMBOL = 1;
    
    // Trade Indices
    private static final int IDX_TRADE_PRICE = 2;
    private static final int IDX_TRADE_VOLUME = 3;
    private static final int IDX_TRADE_TIME = 4;

    // Summary Indices (Sin dayVolume como advirtió el arquitecto)
    private static final int IDX_SUMM_OPEN = 2;
    private static final int IDX_SUMM_HIGH = 3;
    private static final int IDX_SUMM_LOW = 4;
    private static final int IDX_SUMM_PREV_CLOSE = 5;
    private static final int IDX_SUMM_OI = 6;

    // Profile Indices
    private static final int IDX_PROF_SHARES = 2;
    private static final int IDX_PROF_EPS = 3;
    private static final int IDX_PROF_DIV_AMT = 4;
    private static final int IDX_PROF_DIV_FREQ = 5;
    private static final int IDX_PROF_STATUS = 6;
    private static final int IDX_PROF_STATUS_RSN = 7;
    private static final int IDX_PROF_HALT_START = 8;
    private static final int IDX_PROF_HALT_END = 9;
    private static final int IDX_PROF_BETA = 10;
    private static final int IDX_PROF_FLOAT = 11;

    // TradeETH Indices
    private static final int IDX_ETH_VOL = 2;
    private static final int IDX_ETH_IS_EXT = 3;
    private static final int IDX_ETH_TIME = 4;

    // Greeks Indices
    private static final int IDX_GRK_IV = 2;
    private static final int IDX_GRK_DELTA = 3;
    private static final int IDX_GRK_GAMMA = 4;
    private static final int IDX_GRK_THETA = 5;
    private static final int IDX_GRK_VEGA = 6;
    private static final int IDX_GRK_RHO = 7;
    private static final int IDX_GRK_THEO = 8;

    public interface CandleCallback {
        void onCandle(String symbol, Candle candle, boolean isSnapshotComplete);
    }

    // --- Métodos de Configuración Global (Delegados al Default Channel) ---

    public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> callback) {
        if (defaultChannel != null)
            defaultChannel.setOnMarketData(callback);
    }

    public void setOnCandle(CandleCallback callback) {
        if (defaultChannel != null)
            defaultChannel.addCandleListener(callback);
    }

    public void setOnMessage(BiConsumer<String, JsonNode> callback) {
        if (defaultChannel != null)
            defaultChannel.addMessageListener(callback);
    }

    public void setOnGreeks(BiConsumer<String, OptionContract> callback) {
        if (defaultChannel != null)
            defaultChannel.addGreeksListener(callback);
    }

    public void setTokenRefresher(Supplier<String> tokenRefresher) {
        this.tokenRefresher = tokenRefresher;
    }

    /**
     * Crea y abre un nuevo canal dedicado para operaciones aisladas (ej. Batch
     * Requests).
     */
    public CompletableFuture<DxLinkChannel> openNewChannel() {
        if (!authenticated || session == null || !session.isOpen()) {
            return CompletableFuture.failedFuture(new IllegalStateException("Client not authenticated/connected"));
        }
        int newId = nextChannelId.addAndGet(2);
        DxLinkChannel channel = new DxLinkChannel(newId);
        channels.put(newId, channel);

        return channel.initialize();
    }

    /**
     * Conecta al WebSocket DxLink, autentica e inicializa el canal por defecto.
     */
    public void connect(String url, String token) {
        this.dxLinkUrl = url;
        this.apiQuoteToken = token;
        this.authenticated = false;
        this.reconnectAttempts.set(0);
        this.channels.clear();
        this.nextChannelId.set(-1);

        log.debug("Connecting to DxLink: {}", url);

        try {
            WebSocketContainer container = ContainerProvider.getWebSocketContainer();
            container.setDefaultMaxSessionIdleTimeout(120000);
            container.setDefaultMaxTextMessageBufferSize(65536);

            StandardWebSocketClient client = new StandardWebSocketClient(container);
            WebSocketHttpHeaders headers = new WebSocketHttpHeaders();
            headers.add("User-Agent", "metradingplat/1.0");

            client.execute(new DxLinkHandler(), headers, java.net.URI.create(url)).get(30, TimeUnit.SECONDS);

            // Esperar autenticación
            int waitCount = 0;
            while (!authenticated && waitCount < 100) { // 10s max
                Thread.sleep(100);
                waitCount++;
            }

            if (authenticated) {
                log.info("DxLink conectado y autenticado (url={})", url);

                // Inicializar canal default (ID 1)
                this.defaultChannel = new DxLinkChannel(nextChannelId.addAndGet(2));
                this.channels.put(defaultChannel.getId(), defaultChannel);

                try {
                    defaultChannel.initialize().get(10, TimeUnit.SECONDS);
                    log.info("Default channel (ID={}) configured and ready", defaultChannel.getId());
                } catch (Exception e) {
                    log.error("Failed to initialize default channel", e);
                }

                startHealthCheck();
            } else {
                log.warn("DxLink auth timeout after 10s");
                if (session != null && session.isOpen())
                    session.close();
                scheduleReconnect();
            }
        } catch (Exception e) {
            log.error("Failed to connect to DxLink", e);
            scheduleReconnect();
        }
    }

    private void scheduleReconnect() {
        if (!reconnecting.compareAndSet(false, true))
            return;

        int attempts = reconnectAttempts.incrementAndGet();
        if (attempts > MAX_RECONNECT_ATTEMPTS) {
            log.error("Max reconnection attempts reached.");
            reconnecting.set(false);
            return;
        }

        int delaySeconds = Math.min(INITIAL_RECONNECT_DELAY_SECONDS * (int) Math.pow(2, attempts - 1),
                MAX_RECONNECT_DELAY_SECONDS);
        log.debug("Scheduling reconnection attempt {} in {} seconds...", attempts, delaySeconds);

        scheduler.schedule(() -> {
            try {
                performReconnect();
            } finally {
                reconnecting.set(false);
            }
        }, delaySeconds, TimeUnit.SECONDS);
    }

    private void performReconnect() {
        log.debug("Attempting to reconnect...");
        cleanupConnection();

        String freshToken = apiQuoteToken;
        if (tokenRefresher != null) {
            try {
                freshToken = tokenRefresher.get();
            } catch (Exception e) {
                log.error("Token refresh failed", e);
                scheduleReconnect();
                return;
            }
        }

        try {
            connect(dxLinkUrl, freshToken);
            if (authenticated) {
                log.info("DxLink reconnected. Resubscribing {} symbols...", subscribedSymbols.size());
                resubscribeAll();
            }
        } catch (Exception e) {
            scheduleReconnect();
        }
    }

    private void resubscribeAll() {
        if (defaultChannel == null || !defaultChannel.isReady())
            return;

        Set<String> symbols = Set.copyOf(subscribedSymbols);
        for (String symbol : symbols) {
            defaultChannel.subscribe(symbol);
        }
    }

    private void cleanupConnection() {
        authenticated = false;
        channels.clear();
        if (keepaliveTask != null) {
            keepaliveTask.cancel(false);
            keepaliveTask = null;
        }
        if (session != null && session.isOpen()) {
            try {
                session.close();
            } catch (Exception e) {
                /* ignore */ }
        }
        session = null;
    }

    private void startHealthCheck() {
        if (healthCheckTask != null)
            healthCheckTask.cancel(false);
        healthCheckTask = scheduler.scheduleAtFixedRate(this::checkConnectionHealth,
                HEALTH_CHECK_INTERVAL_SECONDS, HEALTH_CHECK_INTERVAL_SECONDS, TimeUnit.SECONDS);
    }

    private void checkConnectionHealth() {
        boolean healthy = session != null && session.isOpen() && authenticated
                && (defaultChannel != null && defaultChannel.isReady());
        if (!healthy) {
            log.warn("Health check failed. Triggering reconnect...");
            scheduleReconnect();
        }
    }

    // --- Métodos de Suscripción (Delegados al Default Channel) ---

    public void subscribe(String symbol) {
        if (defaultChannel != null)
            defaultChannel.subscribe(symbol);
        subscribedSymbols.add(symbol);
    }

    public void subscribe(List<String> symbols) {
        if (defaultChannel != null) {
            symbols.forEach(this::subscribe);
        }
    }

    public void unsubscribe(String symbol) {
        if (defaultChannel != null)
            defaultChannel.unsubscribe(symbol);
        subscribedSymbols.remove(symbol);
    }

    // --- Gestión de Conexión ---

    public void disconnect() {
        cleanupConnection();
        if (healthCheckTask != null)
            healthCheckTask.cancel(true);
        scheduler.shutdown();
    }

    public boolean isConnected() {
        return session != null && session.isOpen() && authenticated;
    }

    public void forceReconnect() {
        reconnectAttempts.set(0);
        cleanupConnection();
        scheduleReconnect();
    }

    public Map<String, Object> getConnectionStats() {
        return Map.of(
                "connected", isConnected(),
                "authenticated", authenticated,
                "channels", channels.size(),
                "activeSubscriptions", subscribedSymbols.size(),
                "reconnectAttempts", reconnectAttempts.get(),
                "reconnecting", reconnecting.get());
    }

    @PreDestroy
    public void cleanup() {
        disconnect();
    }

    // --- WebSocket Logic ---

    private final Object sendLock = new Object();

    private void sendMessage(Object message) {
        try {
            if (session == null || !session.isOpen())
                return;
            String json = objectMapper.writeValueAsString(message);

            if (!json.contains("KEEPALIVE")) {
                log.debug(">>> Sending: {}", json);
            }

            synchronized (sendLock) {
                session.sendMessage(new TextMessage(json));
            }
        } catch (Exception e) {
            log.error("Failed to send message", e);
        }
    }

    private void handleMessage(String payload) {
        try {
            JsonNode msg = objectMapper.readTree(payload);
            String type = msg.path("type").asText();

            switch (type) {
                case "SETUP" -> handleSetup(msg);
                case "AUTH_STATE" -> handleAuthState(msg);
                case "CHANNEL_OPENED" -> handleChannelOpened(msg);
                case "CHANNEL_CLOSED" -> log.debug("Channel {} closed by server", msg.path("channel").asInt());
                case "FEED_CONFIG" -> handleFeedConfig(msg);
                case "FEED_DATA" -> handleFeedData(msg);
                case "KEEPALIVE" -> handleKeepalive();
                case "ERROR" -> handleError(msg);
                default -> log.debug("Unhandled message type: {}", type);
            }
        } catch (Exception e) {
            log.error("Error processing message", e);
        }
    }

    private void handleSetup(JsonNode msg) {
        sendMessage(Map.of("type", "AUTH", "channel", 0, "token", apiQuoteToken));
    }

    private void handleAuthState(JsonNode msg) {
        String state = msg.path("state").asText();
        if ("AUTHORIZED".equals(state)) {
            authenticated = true;
            log.info("Authenticated successfully");
        } else {
            log.debug("Auth state received: {} (waiting for AUTHORIZED)", state);
        }
    }

    private void handleChannelOpened(JsonNode msg) {
        int channelId = msg.path("channel").asInt();
        DxLinkChannel channel = channels.get(channelId);
        if (channel != null && "FEED".equals(msg.path("service").asText())) {
            channel.handleOpened();
        }
    }

    private void handleFeedConfig(JsonNode msg) {
        int channelId = msg.path("channel").asInt();
        DxLinkChannel channel = channels.get(channelId);
        if (channel != null) {
            channel.handleConfigured();
        }
    }

    private void handleFeedData(JsonNode msg) {
        int channelId = msg.path("channel").asInt();
        JsonNode data = msg.path("data");

        // Si el channelId es 0 o no viene, y solo hay un canal default, asumimos es
        // para ese.
        DxLinkChannel channel = channels.get(channelId);
        if (channel == null && channels.size() == 1) {
            channel = channels.values().iterator().next();
        }

        if (channel != null && data.isArray()) {
            channel.processData(data);
        }
    }

    private void handleError(JsonNode msg) {
        String errorCode = msg.path("error").asText();
        String errorMessage = msg.path("message").asText("");
        int channelId = msg.path("channel").asInt(0);
        if (channelId > 0) {
            DxLinkChannel ch = channels.get(channelId);
            if (ch != null) {
                log.error("DxLink error on channel {}: {} — {}", channelId, errorCode, errorMessage);
                ch.failInitialization(errorCode);
            } else {
                log.error("DxLink error: {} (channel {} not found) — {}", errorCode, channelId, errorMessage);
            }
        } else if ("BAD_ACTION".equals(errorCode)) {
            log.error("DxLink BAD_ACTION (no channel) — {} — failing pending channels", errorMessage);
            List.copyOf(channels.values()).stream()
                    .filter(ch -> !ch.initFuture.isDone())
                    .forEach(ch -> ch.failInitialization(errorCode));
        } else {
            log.error("DxLink error (no channel): {} — {}", errorCode, errorMessage);
        }
    }

    private void handleKeepalive() {
        // Echo keepalive
        sendMessage(Map.of("type", "KEEPALIVE", "channel", 0));
    }

    private void startKeepalive() {
        if (keepaliveTask != null) {
            keepaliveTask.cancel(true);
        }
        keepaliveTask = scheduler.scheduleAtFixedRate(
                () -> {
                    try {
                        // Keepalive solo para el default channel o global
                        if (session != null && session.isOpen() && authenticated) {
                            // Sending keepalive globally on channel 0
                            sendMessage(Map.of("type", "KEEPALIVE", "channel", 0));
                        }
                    } catch (Exception e) {
                        log.error("Failed to send keepalive", e);
                    }
                },
                30, 30, TimeUnit.SECONDS);
    }

    // --- Inner Class: DxLinkChannel ---

    public class DxLinkChannel implements AutoCloseable {
        private final int id;
        private final CompletableFuture<DxLinkChannel> initFuture = new CompletableFuture<>();
        private volatile boolean ready = false;

        // Estado Snapshot Local
        private final AtomicInteger snapshotCandleCount = new AtomicInteger(0);
        private final Set<BiConsumer<String, MarketDataStreamDTO>> marketDataListeners = ConcurrentHashMap.newKeySet();

        public void addCandleListener(CandleCallback listener) {
            DxLinkClient.this.candleListeners.add(listener);
        }

        public void removeCandleListener(CandleCallback listener) {
            DxLinkClient.this.candleListeners.remove(listener);
        }

        public void addFundamentalListener(BiConsumer<String, FundamentalData> listener) {
            DxLinkClient.this.fundamentalListeners.add(listener);
        }

        public void addGreeksListener(BiConsumer<String, OptionContract> listener) {
            DxLinkClient.this.greeksListeners.add(listener);
        }

        private void notifyFundamentalListeners(String symbol, FundamentalData data) {
            for (BiConsumer<String, FundamentalData> listener : fundamentalListeners) {
                try {
                    listener.accept(symbol, data);
                } catch (Exception e) {
                    log.error("Error in fundamental listener", e);
                }
            }
        }

        public void addMessageListener(BiConsumer<String, JsonNode> listener) {
            DxLinkClient.this.messageListeners.add(listener);
        }

        public void addMarketDataListener(BiConsumer<String, MarketDataStreamDTO> listener) {
            marketDataListeners.add(listener);
        }

        public void removeMarketDataListener(BiConsumer<String, MarketDataStreamDTO> listener) {
            marketDataListeners.remove(listener);
        }

        public DxLinkChannel(int id) {
            this.id = id;
        }

        public int getId() {
            return id;
        }

        public boolean isReady() {
            return ready;
        }

        public void setOnCandle(CandleCallback cb) {
            candleListeners.clear();
            candleListeners.add(cb);
        }

        public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> cb) {
            marketDataListeners.clear();
            marketDataListeners.add(cb);
        }

        public CompletableFuture<DxLinkChannel> initialize() {
            sendMessage(Map.of(
                    "type", "CHANNEL_REQUEST",
                    "channel", id,
                    "service", "FEED",
                    "parameters", Map.of("contract", "AUTO")));
            return initFuture;
        }

        public void subscribe(String symbol) {
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id,
                    "add",
                    List.of(
                        Map.of("symbol", symbol, "type", "Quote"), 
                        Map.of("symbol", symbol, "type", "Trade"),
                        Map.of("symbol", symbol, "type", "Message")
                    )));
        }

        public void unsubscribe(String symbol) {
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id,
                    "remove",
                    List.of(
                        Map.of("symbol", symbol, "type", "Quote"), 
                        Map.of("symbol", symbol, "type", "Trade"),
                        Map.of("symbol", symbol, "type", "Summary"),
                        Map.of("symbol", symbol, "type", "Profile"),
                        Map.of("symbol", symbol, "type", "Message")
                    )));
        }

        public void subscribeFundamentals(String symbol) {
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id,
                    "add",
                    List.of(
                        Map.of("symbol", symbol, "type", "Summary"),
                        Map.of("symbol", symbol, "type", "Profile")
                    )));
        }

        public void subscribeFundamentalsBatch(List<String> symbols) {
            List<Map<String, Object>> items = new java.util.ArrayList<>();
            for (String s : symbols) {
                items.add(Map.of("symbol", s, "type", "Summary"));
                items.add(Map.of("symbol", s, "type", "Profile"));
                items.add(Map.of("symbol", s, "type", "TradeETH"));
            }
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "add", items));
        }

        public void subscribeExtendedVolumeBatch(List<String> symbols) {
            List<Map<String, Object>> items = new java.util.ArrayList<>();
            for (String s : symbols) {
                items.add(Map.of("symbol", s, "type", "TradeETH"));
            }
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "add", items));
        }

        public void subscribeCandlesBatch(List<Map<String, Object>> items) {
            snapshotCandleCount.set(0); // Reset local count
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "add", items));
        }

        public void subscribeCandlesHistory(List<Map<String, Object>> items, long fromTime) {
            snapshotCandleCount.set(0);
            
            // Según la documentación, fromTime debe ir DENTRO de cada item
            List<Map<String, Object>> itemsWithTime = items.stream()
                .map(item -> {
                    Map<String, Object> newItem = new java.util.HashMap<>(item);
                    newItem.put("fromTime", fromTime);
                    return newItem;
                })
                .toList();

            sendMessage(Map.of(
                "type", "FEED_SUBSCRIPTION", 
                "channel", id, 
                "add", itemsWithTime
            ));
        }

        public void close() {
            // Enviar CHANNEL_CANCEL para cerrar el canal en el servidor.
            sendMessage(Map.of("type", "CHANNEL_CANCEL", "channel", id));
            channels.remove(id);
        }

        void failInitialization(String reason) {
            initFuture.completeExceptionally(new RuntimeException("DxLink channel error: " + reason));
            channels.remove(id);
        }

        private void handleOpened() {
            sendMessage(Map.of(
                    "type", "FEED_SETUP",
                    "channel", id,
                    "acceptAggregationPeriod", 0.1, // Throttle de 100ms para no saturar la red
                    "acceptDataFormat", "COMPACT",
                    "acceptEventFields", Map.of(
                            "Quote", QUOTE_FIELDS,
                            "Trade", TRADE_FIELDS,
                            "Summary", SUMMARY_FIELDS,
                            "Profile", PROFILE_FIELDS,
                            "TradeETH", TRADE_ETH_FIELDS,
                            "Greeks", GREEKS_FIELDS,
                            "Message", List.of("eventSymbol", "eventTime", "messageType", "message"),
                            "Candle", List.of("eventSymbol", "time", "open", "high", "low", "close", "volume", "eventFlags"))));
        }

        private void handleConfigured() {
            this.ready = true;
            initFuture.complete(this);

            // Iniciar keepalive solo si es el canal default
            if (this == defaultChannel) {
                DxLinkClient.this.startKeepalive();
            }
        }

        private void processData(JsonNode data) {
            int i = 0;
            while (i < data.size()) {
                JsonNode item = data.get(i);
                if (item.isTextual()) {
                    String eventType = item.asText();
                    if (i + 1 < data.size()) {
                        i++;
                        processCompactEvent(eventType, data.get(i));
                    }
                } else if (item.isObject()) {
                    String eventType = item.has("eventType") ? item.get("eventType").asText()
                            : item.get("type").asText();
                    processFullEvent(eventType, item);
                }
                i++;
            }
        }

        private void processCompactEvent(String eventType, JsonNode data) {
            try {
                switch (eventType) {
                    case "Quote" -> {
                        String symbol = data.path(0).asText();
                        MarketDataStreamDTO streamData = MarketDataStreamDTO.builder()
                                .symbol(symbol)
                                .bid(data.path(1).asDouble())
                                .ask(data.path(2).asDouble())
                                .timestamp(Instant.now())
                                .build();
                        for (BiConsumer<String, MarketDataStreamDTO> listener : marketDataListeners) {
                            try {
                                listener.accept(symbol, streamData);
                            } catch (Exception e) {
                                log.error("Error in market data listener (Quote)", e);
                            }
                        }
                    }
                    case "Trade" -> {
                        String symbol = data.path(0).asText();
                        MarketDataStreamDTO streamData = MarketDataStreamDTO.builder()
                                .symbol(symbol)
                                .lastPrice(data.path(1).asDouble())
                                .volume(data.path(2).asLong())
                                .timestamp(Instant.ofEpochMilli(data.path(3).asLong()))
                                .build();
                        for (BiConsumer<String, MarketDataStreamDTO> listener : marketDataListeners) {
                            try {
                                listener.accept(symbol, streamData);
                            } catch (Exception e) {
                                log.error("Error in market data listener (Trade)", e);
                            }
                        }
                    }
                    case "Summary" -> {
                        // Order: eventSymbol, dayOpenPrice, dayHighPrice, dayLowPrice, prevDayClosePrice, prevDayVolume, openInterest
                        String symbol = data.path(0).asText();
                        FundamentalData fundData = FundamentalData.builder()
                                .symbol(symbol)
                                .open(data.path(1).asDouble())
                                .high(data.path(2).asDouble())
                                .low(data.path(3).asDouble())
                                .prevClose(data.path(4).asDouble())
                                .dayVolume(data.path(5).asLong())
                                .openInterest(data.path(6).asLong())
                                .build();
                        log.debug("DxLink Summary event: {}", fundData);
                        notifyFundamentalListeners(symbol, fundData);
                    }
    private void processCompactEvent(String eventType, JsonNode data) {
        try {
            switch (eventType) {
                case "Quote" -> {
                    String symbol = data.path(IDX_SYMBOL).asText();
                    MarketDataStreamDTO streamData = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .bidPrice(data.path(2).asDouble())
                            .askPrice(data.path(3).asDouble())
                            .build();
                    notifyMarketDataListeners(symbol, streamData);
                }
                case "Trade" -> {
                    String symbol = data.path(IDX_SYMBOL).asText();
                    MarketDataStreamDTO streamData = MarketDataStreamDTO.builder()
                            .symbol(symbol)
                            .lastPrice(data.path(IDX_TRADE_PRICE).asDouble())
                            .volume(data.path(IDX_TRADE_VOLUME).asLong())
                            .timestamp(Instant.ofEpochMilli(data.path(IDX_TRADE_TIME).asLong()))
                            .build();
                    notifyMarketDataListeners(symbol, streamData);
                }
                case "Summary" -> {
                    String symbol = data.path(IDX_SYMBOL).asText();
                    FundamentalData fundData = FundamentalData.builder()
                            .symbol(symbol)
                            .open(data.path(IDX_SUMM_OPEN).asDouble())
                            .high(data.path(IDX_SUMM_HIGH).asDouble())
                            .low(data.path(IDX_SUMM_LOW).asDouble())
                            .prevClose(data.path(IDX_SUMM_PREV_CLOSE).asDouble())
                            .openInterest(data.path(IDX_SUMM_OI).asLong())
                            .build();
                    notifyFundamentalListeners(symbol, fundData);
                }
                case "TradeETH" -> {
                    String symbol = data.path(IDX_SYMBOL).asText();
                    long extVol = data.path(IDX_ETH_VOL).asLong();
                    long time = data.path(IDX_ETH_TIME).asLong();
                    
                    FundamentalData fundData = FundamentalData.builder().symbol(symbol).build();
                    // Inferencia de Sesión Pre/Post Market (Basado en 09:30 AM EST)
                    if (isPreMarket(time)) {
                        fundData.setPreMarketVolume(extVol);
                    } else {
                        fundData.setPostMarketVolume(extVol);
                    }
                    notifyFundamentalListeners(symbol, fundData);
                }
                case "Profile" -> {
                    String symbol = data.path(IDX_SYMBOL).asText();
                    FundamentalData fundData = FundamentalData.builder()
                            .symbol(symbol)
                            .sharesOutstanding(data.path(IDX_PROF_SHARES).asLong())
                            .eps(data.path(IDX_PROF_EPS).asDouble())
                            .dividendAmount(data.path(IDX_PROF_DIV_AMT).asDouble())
                            .dividendFrequency(data.path(IDX_PROF_DIV_FREQ).asText())
                            .tradingStatus(data.path(IDX_PROF_STATUS).asText())
                            .statusReason(data.path(IDX_PROF_STATUS_RSN).asText())
                            .haltStartTime(data.path(IDX_PROF_HALT_START).asLong())
                            .haltEndTime(data.path(IDX_PROF_HALT_END).asLong())
                            .beta(data.path(IDX_PROF_BETA).asDouble())
                            .floatShares(data.path(IDX_PROF_FLOAT).asLong())
                            .build();
                    notifyFundamentalListeners(symbol, fundData);
                }
                case "Greeks" -> {
                    String symbol = data.path(IDX_SYMBOL).asText();
                    OptionContract greeks = OptionContract.builder()
                            .symbol(symbol)
                            .impliedVolatility(data.path(IDX_GRK_IV).asDouble())
                            .delta(data.path(IDX_GRK_DELTA).asDouble())
                            .gamma(data.path(IDX_GRK_GAMMA).asDouble())
                            .theta(data.path(IDX_GRK_THETA).asDouble())
                            .vega(data.path(IDX_GRK_VEGA).asDouble())
                            .rho(data.path(IDX_GRK_RHO).asDouble())
                            .theoreticalPrice(data.path(IDX_GRK_THEO).asDouble())
                            .build();
                    for (BiConsumer<String, OptionContract> listener : greeksListeners) {
                        try { listener.accept(symbol, greeks); } catch (Exception e) { log.error("Error in greeks listener", e); }
                    }
                }
                case "Candle" -> {
                    int fieldsPerCandle = 8;
                    for (int idx = 0; idx < data.size(); idx += fieldsPerCandle) {
                        String candleSymbol = data.path(idx).asText();
                        String baseSymbol = candleSymbol.contains("{")
                                ? candleSymbol.substring(0, candleSymbol.indexOf("{"))
                                : candleSymbol;

                        Candle candle = Candle.builder()
                                .symbol(baseSymbol)
                                .timestamp(Instant.ofEpochMilli(data.path(idx + 1).asLong()))
                                .open(data.path(idx + 2).asDouble())
                                .high(data.path(idx + 3).asDouble())
                                .low(data.path(idx + 4).asDouble())
                                .close(data.path(idx + 5).asDouble())
                                .volume(data.path(idx + 6).asDouble())
                                .build();
                        boolean isComplete = (data.path(idx + 7).asInt() & 0x01) == 0;
                        for (CandleCallback listener : candleListeners) {
                            try { listener.onCandle(baseSymbol, candle, isComplete); } catch (Exception e) { log.error("Error in candle listener", e); }
                        }
                    }
                }
                case "Message" -> {
                    String symbol = data.path(0).asText();
                    for (BiConsumer<String, JsonNode> listener : messageListeners) {
                        try { listener.accept(symbol, data); } catch (Exception e) { log.error("Error in message listener", e); }
                    }
                }
            }
        } catch (Exception e) {
            log.error("Error processing compact event {}", eventType, e);
        }
    }

    private boolean isPreMarket(long timeMs) {
        java.time.ZonedDateTime nyTime = Instant.ofEpochMilli(timeMs).atZone(java.time.ZoneId.of("America/New_York"));
        int hour = nyTime.getHour();
        int minute = nyTime.getMinute();
        return (hour < 9) || (hour == 9 && minute < 30);
    }

    private void notifyMarketDataListeners(String symbol, MarketDataStreamDTO data) {
        for (BiConsumer<String, MarketDataStreamDTO> listener : marketDataListeners) {
            try { listener.accept(symbol, data); } catch (Exception e) { log.error("Error in market data listener", e); }
        }
    }

    private void notifyFundamentalListeners(String symbol, FundamentalData data) {
        for (BiConsumer<String, FundamentalData> listener : fundamentalListeners) {
            try { listener.accept(symbol, data); } catch (Exception e) { log.error("Error in fundamental listener", e); }
        }
    }

        private void processFullEvent(String eventType, JsonNode data) {
            // Implementación simplificada para FULL events si fuera necesario
            // Generalmente DxLink con acceptDataFormat=COMPACT usa el otro método.
        }
    }

    private class DxLinkHandler extends TextWebSocketHandler {
        @Override
        public void afterConnectionEstablished(WebSocketSession wsSession) {
            DxLinkClient.this.session = wsSession;
            try {
                String setupJson = objectMapper.writeValueAsString(Map.of(
                        "type", "SETUP", "channel", 0, "version", "0.1-js/1.0.0",
                        "keepaliveTimeout", 60, "acceptKeepaliveTimeout", 60));
                wsSession.sendMessage(new TextMessage(setupJson));
            } catch (Exception e) {
                log.error("Failed to send SETUP", e);
            }
        }

        @Override
        protected void handleTextMessage(WebSocketSession session, TextMessage message) {
            DxLinkClient.this.handleMessage(message.getPayload());
        }

        @Override
        public void afterConnectionClosed(WebSocketSession session, CloseStatus status) {
            authenticated = false;
            channels.clear();
            if (status.getCode() != CloseStatus.NORMAL.getCode()) {
                scheduleReconnect();
            }
        }
    }
}
