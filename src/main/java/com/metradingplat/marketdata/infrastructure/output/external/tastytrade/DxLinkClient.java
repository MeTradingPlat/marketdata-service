package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.time.Instant;
import java.util.ArrayList;
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
 * Implementa el protocolo dxLink WebSocket 1.0.2 con soporte para múltiples canales.
 */
@Component
@Slf4j
public class DxLinkClient {

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final Set<String> subscribedSymbols = ConcurrentHashMap.newKeySet();
    private final ScheduledExecutorService scheduler = Executors.newScheduledThreadPool(2);
    
    // L1 Cache to maintain the last known price during market data silence (OOH)
    private final Map<String, Double> lastKnownPriceCache = new ConcurrentHashMap<>();

    private WebSocketSession session;
    private String apiQuoteToken;
    private String dxLinkUrl;

    private final Map<Integer, DxLinkChannel> channels = new ConcurrentHashMap<>();
    private final AtomicInteger nextChannelId = new AtomicInteger(-1);
    private DxLinkChannel defaultChannel;
    
    private final java.util.concurrent.atomic.AtomicLong messageCounter = new java.util.concurrent.atomic.AtomicLong(0);
    private final long startTime = System.currentTimeMillis();

    private volatile boolean authenticated = false;
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

    // --- Definición de Contratos para Formato COMPACT ---
    private static final List<String> QUOTE_FIELDS = List.of("eventSymbol", "bidPrice", "askPrice");
    private static final List<String> TRADE_FIELDS = List.of("eventSymbol", "price", "dayVolume", "time");
    private static final List<String> SUMMARY_FIELDS = List.of("eventSymbol", "dayOpenPrice", "dayHighPrice", "dayLowPrice", "prevDayClosePrice", "openInterest");
    private static final List<String> PROFILE_FIELDS = List.of("eventSymbol", "shares", "earningsPerShare", "exDividendAmount", "dividendFrequency", "tradingStatus", "statusReason", "haltStartTime", "haltEndTime", "beta", "freeFloat");
    private static final List<String> TRADE_ETH_FIELDS = List.of("eventSymbol", "dayVolume", "extendedTradingHours", "time");
    private static final List<String> GREEKS_FIELDS = List.of("eventSymbol", "volatility", "delta", "gamma", "theta", "vega", "rho", "theoreticalPrice");
    private static final List<String> CANDLE_FIELDS = List.of("eventSymbol", "time", "open", "high", "low", "close", "volume", "VWAP", "impVolatility");

    private static final int IDX_SYMBOL = 0;
    private static final int IDX_TRADE_PRICE = 1;
    private static final int IDX_TRADE_VOLUME = 2;
    private static final int IDX_TRADE_TIME = 3;
    private static final int IDX_SUMM_OPEN = 1;
    private static final int IDX_SUMM_HIGH = 2;
    private static final int IDX_SUMM_LOW = 3;
    private static final int IDX_SUMM_PREV_CLOSE = 4;
    private static final int IDX_SUMM_OI = 5;
    private static final int IDX_PROF_SHARES = 1;
    private static final int IDX_PROF_EPS = 2;
    private static final int IDX_PROF_DIV_AMT = 3;
    private static final int IDX_PROF_DIV_FREQ = 4;
    private static final int IDX_PROF_STATUS = 5;
    private static final int IDX_PROF_STATUS_RSN = 6;
    private static final int IDX_PROF_HALT_START = 7;
    private static final int IDX_PROF_HALT_END = 8;
    private static final int IDX_PROF_BETA = 9;
    private static final int IDX_PROF_FLOAT = 10;
    private static final int IDX_ETH_VOL = 1;
    private static final int IDX_ETH_TIME = 3;
    private static final int IDX_GRK_IV = 1;
    private static final int IDX_GRK_DELTA = 2;
    private static final int IDX_GRK_GAMMA = 3;
    private static final int IDX_GRK_THETA = 4;
    private static final int IDX_GRK_VEGA = 5;
    private static final int IDX_GRK_RHO = 6;
    private static final int IDX_GRK_THEO = 7;
    private static final int IDX_CAND_TIME = 1;
    private static final int IDX_CAND_OPEN = 2;
    private static final int IDX_CAND_HIGH = 3;
    private static final int IDX_CAND_LOW = 4;
    private static final int IDX_CAND_CLOSE = 5;
    private static final int IDX_CAND_VOL = 6;
    private static final int IDX_CAND_VWAP = 7;
    private static final int IDX_CAND_IV = 8;

    public interface CandleCallback {
        void onCandle(String symbol, Candle candle, boolean isSnapshotComplete);
    }

    public DxLinkChannel getDefaultChannel() { return defaultChannel; }

    public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> callback) { if (defaultChannel != null) defaultChannel.setOnMarketData(callback); }
    public void setOnCandle(CandleCallback callback) { if (defaultChannel != null) defaultChannel.addCandleListener(callback); }
    public void setOnMessage(BiConsumer<String, JsonNode> callback) { if (defaultChannel != null) defaultChannel.addMessageListener(callback); }
    public void setOnGreeks(BiConsumer<String, OptionContract> callback) { if (defaultChannel != null) defaultChannel.addGreeksListener(callback); }
    private BiConsumer<String, FundamentalData> fundamentalCallback;

    public void setOnFundamentalData(BiConsumer<String, FundamentalData> callback) {
        this.fundamentalCallback = callback;
        if (defaultChannel != null) defaultChannel.addFundamentalListener(callback);
    }
    public void setTokenRefresher(Supplier<String> tokenRefresher) { this.tokenRefresher = tokenRefresher; }

    public CompletableFuture<DxLinkChannel> openNewChannel() {
        if (!authenticated || session == null || !session.isOpen()) return CompletableFuture.failedFuture(new IllegalStateException("Not connected"));
        int newId = nextChannelId.addAndGet(2);
        DxLinkChannel channel = new DxLinkChannel(newId);
        channels.put(newId, channel);
        return channel.initialize();
    }

    public void connect(String url, String token) {
        this.dxLinkUrl = url;
        this.apiQuoteToken = token;
        cleanupConnection();
        try {
            WebSocketContainer container = ContainerProvider.getWebSocketContainer();
            container.setDefaultMaxSessionIdleTimeout(120000);
            StandardWebSocketClient client = new StandardWebSocketClient(container);
            WebSocketHttpHeaders headers = new WebSocketHttpHeaders();
            headers.add("User-Agent", "QuantMaestro/2.0.0");
            client.execute(new DxLinkHandler(), headers, java.net.URI.create(url)).get(30, TimeUnit.SECONDS);

            int waitCount = 0;
            while (!authenticated && waitCount < 100) { Thread.sleep(100); waitCount++; }
            if (authenticated) {
                this.defaultChannel = new DxLinkChannel(nextChannelId.addAndGet(2));
                this.channels.put(defaultChannel.getId(), defaultChannel);
                defaultChannel.initialize().get(10, TimeUnit.SECONDS);
                startHealthCheck();
            } else {
                scheduleReconnect();
            }
        } catch (Exception e) {
            log.error("Failed to connect", e);
            scheduleReconnect();
        }
    }

    private void scheduleReconnect() {
        if (!reconnecting.compareAndSet(false, true)) return;
        int attempts = reconnectAttempts.incrementAndGet();
        if (attempts > MAX_RECONNECT_ATTEMPTS) return;
        int delay = Math.min(INITIAL_RECONNECT_DELAY_SECONDS * (int) Math.pow(2, attempts - 1), MAX_RECONNECT_DELAY_SECONDS);
        scheduler.schedule(() -> { try { performReconnect(); } finally { reconnecting.set(false); } }, delay, TimeUnit.SECONDS);
    }

    private void performReconnect() {
        cleanupConnection();
        String token = (tokenRefresher != null) ? tokenRefresher.get() : apiQuoteToken;
        try {
            connect(dxLinkUrl, token);
            if (authenticated) {
                if (fundamentalCallback != null && defaultChannel != null) {
                    defaultChannel.addFundamentalListener(fundamentalCallback);
                }
                resubscribeAll();
            }
        } catch (Exception e) { scheduleReconnect(); }
    }

    private void resubscribeAll() {
        if (defaultChannel == null || !defaultChannel.isReady()) return;
        List<String> symbols = new ArrayList<>(subscribedSymbols);
        int chunkSize = 33;
        for (int i = 0; i < symbols.size(); i += chunkSize) {
            int end = Math.min(i + chunkSize, symbols.size());
            defaultChannel.subscribeBatch(symbols.subList(i, end));
            if (end < symbols.size()) {
                try { Thread.sleep(500); } catch (InterruptedException e) { Thread.currentThread().interrupt(); break; }
            }
        }
    }

    private void cleanupConnection() {
        authenticated = false;
        channels.clear();
        if (keepaliveTask != null) keepaliveTask.cancel(false);
        if (session != null && session.isOpen()) try { session.close(); } catch (Exception e) {}
        session = null;
    }

    private void startHealthCheck() {
        if (healthCheckTask != null) healthCheckTask.cancel(false);
        healthCheckTask = scheduler.scheduleAtFixedRate(this::checkConnectionHealth, HEALTH_CHECK_INTERVAL_SECONDS, HEALTH_CHECK_INTERVAL_SECONDS, TimeUnit.SECONDS);
    }

    private void checkConnectionHealth() {
        if (session == null || !session.isOpen() || !authenticated || defaultChannel == null || !defaultChannel.isReady()) scheduleReconnect();
    }

    public void subscribe(String symbol) { if (defaultChannel != null) defaultChannel.subscribe(symbol); subscribedSymbols.add(symbol); }
    public void subscribe(List<String> symbols) { if (defaultChannel != null) symbols.forEach(this::subscribe); }
    public void subscribeBatch(List<String> symbols) {
        if (defaultChannel == null || !defaultChannel.isReady()) return;
        defaultChannel.subscribeBatch(symbols);
        subscribedSymbols.addAll(symbols);
    }
    public void unsubscribe(String symbol) { if (defaultChannel != null) defaultChannel.unsubscribe(symbol); subscribedSymbols.remove(symbol); }
    public int getActiveSubscriptionCount() { return subscribedSymbols.size(); }
    public boolean isConnected() { return session != null && session.isOpen() && authenticated; }

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
                "reconnecting", reconnecting.get(),
                "totalMessages", messageCounter.get(),
                "avgMessagesPerSecond", calculateAvgThroughput());
    }

    private double calculateAvgThroughput() {
        long elapsedSeconds = (System.currentTimeMillis() - startTime) / 1000;
        return elapsedSeconds > 0 ? (double) messageCounter.get() / elapsedSeconds : 0;
    }

    @PreDestroy
    public void cleanup() { disconnect(); }
    public void disconnect() { cleanupConnection(); if (healthCheckTask != null) healthCheckTask.cancel(true); scheduler.shutdown(); }

    private final Object sendLock = new Object();
    private void sendMessage(Object message) {
        try {
            if (session == null || !session.isOpen()) return;
            String json = objectMapper.writeValueAsString(message);
            synchronized (sendLock) { session.sendMessage(new TextMessage(json)); }
        } catch (Exception e) { log.error("Send failed", e); }
    }

    private void processRawMessage(String payload) {
        messageCounter.incrementAndGet();
        try {
            JsonNode msg = objectMapper.readTree(payload);
            String type = msg.path("type").asText();
            switch (type) {
                case "SETUP" -> sendMessage(Map.of("type", "AUTH", "channel", 0, "token", apiQuoteToken));
                case "AUTH_STATE" -> { if ("AUTHORIZED".equals(msg.path("state").asText())) authenticated = true; }
                case "CHANNEL_OPENED" -> {
                    int chId = msg.path("channel").asInt();
                    DxLinkChannel ch = channels.get(chId);
                    if (ch != null && "FEED".equals(msg.path("service").asText())) ch.handleOpened();
                }
                case "FEED_CONFIG" -> {
                    int chId = msg.path("channel").asInt();
                    DxLinkChannel ch = channels.get(chId);
                    if (ch != null) ch.handleConfigured();
                }
                case "FEED_DATA" -> {
                    int chId = msg.path("channel").asInt();
                    DxLinkChannel ch = channels.get(chId);
                    if (ch != null) ch.processData(msg.path("data"));
                }
                case "KEEPALIVE" -> sendMessage(Map.of("type", "KEEPALIVE", "channel", 0));
                case "ERROR" -> {
                    String errorType = msg.path("error").asText("");
                    if ("BAD_ACTION".equals(errorType)) {
                        log.debug("DxLink BAD_ACTION (informational): {}", msg.path("message").asText());
                    } else {
                        log.error("DxLink error: {}", msg);
                    }
                }
            }
        } catch (Exception e) { log.error("Message error", e); }
    }

    private void startKeepalive() {
        if (keepaliveTask != null) keepaliveTask.cancel(true);
        keepaliveTask = scheduler.scheduleAtFixedRate(() -> { if (isConnected()) sendMessage(Map.of("type", "KEEPALIVE", "channel", 0)); }, 30, 30, TimeUnit.SECONDS);
    }

    public class DxLinkChannel implements AutoCloseable {
        private final int id;
        private final CompletableFuture<DxLinkChannel> initFuture = new CompletableFuture<>();
        private volatile boolean ready = false;
        private final Set<BiConsumer<String, MarketDataStreamDTO>> marketDataListeners = ConcurrentHashMap.newKeySet();

        public DxLinkChannel(int id) { this.id = id; }
        public int getId() { return id; }
        public boolean isReady() { return ready; }

        public void addCandleListener(CandleCallback listener) { candleListeners.add(listener); }
        public void addFundamentalListener(BiConsumer<String, FundamentalData> listener) { fundamentalListeners.add(listener); }
        public void addGreeksListener(BiConsumer<String, OptionContract> listener) { greeksListeners.add(listener); }
        public void addMessageListener(BiConsumer<String, JsonNode> listener) { messageListeners.add(listener); }
        public void addMarketDataListener(BiConsumer<String, MarketDataStreamDTO> listener) { marketDataListeners.add(listener); }
        public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> cb) { marketDataListeners.clear(); marketDataListeners.add(cb); }

        public CompletableFuture<DxLinkChannel> initialize() {
            sendMessage(Map.of("type", "CHANNEL_REQUEST", "channel", id, "service", "FEED", "parameters", Map.of("contract", "AUTO")));
            return initFuture;
        }

        public void subscribe(String symbol) {
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "add", List.of(
                Map.of("symbol", symbol, "type", "Quote"),
                Map.of("symbol", symbol, "type", "Trade"),
                Map.of("symbol", symbol, "type", "TradeETH"),
                Map.of("symbol", symbol, "type", "Summary"),
                Map.of("symbol", symbol, "type", "Profile"),
                Map.of("symbol", symbol, "type", "Message")
            )));
        }

        public void subscribeBatch(List<String> symbols) {
            List<Map<String, Object>> items = new java.util.ArrayList<>();
            for (String s : symbols) {
                items.add(Map.of("symbol", s, "type", "Quote"));
                items.add(Map.of("symbol", s, "type", "Trade"));
                items.add(Map.of("symbol", s, "type", "TradeETH"));
                items.add(Map.of("symbol", s, "type", "Summary"));
                items.add(Map.of("symbol", s, "type", "Profile"));
                items.add(Map.of("symbol", s, "type", "Message"));
            }
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "add", items));
        }

        public void unsubscribe(String symbol) {
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "remove", List.of(
                Map.of("symbol", symbol, "type", "Quote"),
                Map.of("symbol", symbol, "type", "Trade"),
                Map.of("symbol", symbol, "type", "TradeETH"),
                Map.of("symbol", symbol, "type", "Summary"),
                Map.of("symbol", symbol, "type", "Profile"),
                Map.of("symbol", symbol, "type", "Message")
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

        public void subscribeCandlesHistory(String symbol, String timeframe, long fromTime) {
            String candleSymbol = String.format("%s{=%s}", symbol, timeframe);
            log.info("📊 Solicitando Time-Series desde {} para {}", fromTime, candleSymbol);
            
            sendMessage(Map.of(
                "type", "FEED_SUBSCRIPTION", 
                "channel", id, 
                "add", List.of(Map.of(
                    "symbol", candleSymbol, 
                    "type", "Candle", 
                    "fromTime", fromTime
                ))
            ));
        }

        public void subscribeCandlesHistory(List<Map<String, Object>> items, long fromTime) {
            List<Map<String, Object>> itemsWithTime = items.stream().map(item -> { 
                Map<String, Object> ni = new java.util.HashMap<>(item); 
                ni.put("fromTime", fromTime); 
                return ni; 
            }).toList();
            sendMessage(Map.of("type", "FEED_SUBSCRIPTION", "channel", id, "add", itemsWithTime));
        }

        public void close() { sendMessage(Map.of("type", "CHANNEL_CANCEL", "channel", id)); channels.remove(id); }

        private void handleOpened() {
            sendMessage(Map.of("type", "FEED_SETUP", "channel", id, "acceptAggregationPeriod", 1.5, "acceptDataFormat", "COMPACT", "acceptEventFields", Map.of("Quote", QUOTE_FIELDS, "Trade", TRADE_FIELDS, "Summary", SUMMARY_FIELDS, "Profile", PROFILE_FIELDS, "TradeETH", TRADE_ETH_FIELDS, "Greeks", GREEKS_FIELDS, "Message", List.of("eventSymbol", "eventTime", "messageType", "message"), "Candle", CANDLE_FIELDS)));
        }

        private void handleConfigured() { this.ready = true; initFuture.complete(this); if (this == defaultChannel) startKeepalive(); }

        private int eventCount = 0;

        private void processData(JsonNode data) {
            int i = 0;
            while (i < data.size()) {
                JsonNode item = data.get(i);
                if (item.isTextual()) { String type = item.asText(); if (i + 1 < data.size()) processCompactEvent(type, data.get(++i)); }
                i++;
            }
            eventCount++;
            if (eventCount % 100 == 1) log.info("DxLink channel {} received {} events so far", id, eventCount);
        }

        private void processCompactEvent(String type, JsonNode data) {
            try {
                String symbol = data.path(IDX_SYMBOL).asText();
                if (symbol == null || symbol.isEmpty()) return;

                if ("Profile".equals(type)) log.info("DxLink Profile received for {}", symbol);
                if ("Summary".equals(type)) log.debug("DxLink Summary received for {}", symbol);
                if ("Candle".equals(type)) log.info("DxLink Candle received for {} time={}", symbol, data.path(IDX_CAND_TIME).asLong());

                switch (type) {
                    case "Quote" -> {
                        double bid = extractNumericSafe(data.get(1));
                        double ask = extractNumericSafe(data.get(2));
                        notifyMarketData(symbol, MarketDataStreamDTO.builder().symbol(symbol).bid(bid).ask(ask).timestamp(Instant.now()).build());
                    }
                    case "Trade" -> {
                        double price = extractNumericSafe(data.get(IDX_TRADE_PRICE));
                        if (!Double.isNaN(price)) {
                            lastKnownPriceCache.put(symbol, price);
                        } else {
                            price = lastKnownPriceCache.getOrDefault(symbol, Double.NaN);
                        }
                        notifyMarketData(symbol, MarketDataStreamDTO.builder().symbol(symbol).lastPrice(price).volume(data.path(IDX_TRADE_VOLUME).asLong()).timestamp(Instant.ofEpochMilli(data.path(IDX_TRADE_TIME).asLong())).build());
                    }
                    case "Summary" -> notifyFundamentals(symbol, FundamentalData.builder().symbol(symbol).open(extractNumericSafe(data.get(IDX_SUMM_OPEN))).high(extractNumericSafe(data.get(IDX_SUMM_HIGH))).low(extractNumericSafe(data.get(IDX_SUMM_LOW))).prevClose(extractNumericSafe(data.get(IDX_SUMM_PREV_CLOSE))).openInterest(data.path(IDX_SUMM_OI).asLong()).build());
                    case "TradeETH" -> {
                        long v = data.path(IDX_ETH_VOL).asLong(); long t = data.path(IDX_ETH_TIME).asLong();
                        FundamentalData f = FundamentalData.builder().symbol(symbol).build();
                        if (isPreMarket(t)) f.setPreMarketVolume(v); else f.setPostMarketVolume(v);
                        notifyFundamentals(symbol, f);
                    }
                    case "Profile" -> notifyFundamentals(symbol, FundamentalData.builder().symbol(symbol).sharesOutstanding(data.path(IDX_PROF_SHARES).asLong()).eps(extractNumericSafe(data.get(IDX_PROF_EPS))).dividendAmount(extractNumericSafe(data.get(IDX_PROF_DIV_AMT))).dividendFrequency(data.path(IDX_PROF_DIV_FREQ).asText()).tradingStatus(data.path(IDX_PROF_STATUS).asText()).statusReason(data.path(IDX_PROF_STATUS_RSN).asText()).haltStartTime(data.path(IDX_PROF_HALT_START).asLong()).haltEndTime(data.path(IDX_PROF_HALT_END).asLong()).beta(extractNumericSafe(data.get(IDX_PROF_BETA))).floatShares(data.path(IDX_PROF_FLOAT).asLong()).build());
                    case "Greeks" -> notifyGreeks(symbol, OptionContract.builder().symbol(symbol).impliedVolatility(extractNumericSafe(data.get(IDX_GRK_IV))).delta(extractNumericSafe(data.get(IDX_GRK_DELTA))).gamma(extractNumericSafe(data.get(IDX_GRK_GAMMA))).theta(extractNumericSafe(data.get(IDX_GRK_THETA))).vega(extractNumericSafe(data.get(IDX_GRK_VEGA))).rho(extractNumericSafe(data.get(IDX_GRK_RHO))).theoreticalPrice(extractNumericSafe(data.get(IDX_GRK_THEO))).build());
                    case "Candle" -> {
                        long time = data.path(IDX_CAND_TIME).asLong();
                        boolean isComplete = data.path(10).asInt() != 0; // Flags o similar si existen
                        notifyCandle(symbol, Candle.builder()
                                .symbol(symbol)
                                .timestamp(Instant.ofEpochMilli(time))
                                .open(extractNumericSafe(data.get(IDX_CAND_OPEN)))
                                .high(extractNumericSafe(data.get(IDX_CAND_HIGH)))
                                .low(extractNumericSafe(data.get(IDX_CAND_LOW)))
                                .close(extractNumericSafe(data.get(IDX_CAND_CLOSE)))
                                .volume(extractNumericSafe(data.get(IDX_CAND_VOL)))
                                .vwap(extractNumericSafe(data.get(IDX_CAND_VWAP)))
                                .impVolatility(extractNumericSafe(data.get(IDX_CAND_IV)))
                                .build(), isComplete);
                    }
                }
            } catch (Exception e) { log.error("Compact error processing event type: " + type, e); }
        }

        private double extractNumericSafe(JsonNode node) {
            if (node == null || node.isMissingNode() || node.isNull()) {
                return Double.NaN;
            }
            if (node.isNumber()) {
                return node.asDouble();
            }
            // dxLink sends String literals for special IEEE 754 values
            String textVal = node.asText();
            return switch (textVal) {
                case "NaN" -> Double.NaN;
                case "Infinity" -> Double.POSITIVE_INFINITY;
                case "-Infinity" -> Double.NEGATIVE_INFINITY;
                default -> {
                    try { yield Double.parseDouble(textVal); } 
                    catch (NumberFormatException e) { yield Double.NaN; }
                }
            };
        }

        private void notifyMarketData(String s, MarketDataStreamDTO d) { marketDataListeners.forEach(l -> { try { l.accept(s, d); } catch (Exception e) {} }); }
        private void notifyFundamentals(String s, FundamentalData d) { fundamentalListeners.forEach(l -> { try { l.accept(s, d); } catch (Exception e) {} }); }
        private void notifyGreeks(String s, OptionContract d) { greeksListeners.forEach(l -> { try { l.accept(s, d); } catch (Exception e) {} }); }
        private void notifyCandle(String s, Candle c, boolean complete) { candleListeners.forEach(l -> { try { l.onCandle(s, c, complete); } catch (Exception e) {} }); }
        private boolean isPreMarket(long t) { java.time.ZonedDateTime ny = Instant.ofEpochMilli(t).atZone(java.time.ZoneId.of("America/New_York")); int h = ny.getHour(); int m = ny.getMinute(); return (h < 9) || (h == 9 && m < 30); }
        void failInitialization(String r) { initFuture.completeExceptionally(new RuntimeException(r)); channels.remove(id); }
    }

    private class DxLinkHandler extends TextWebSocketHandler {
        @Override public void afterConnectionEstablished(WebSocketSession s) { session = s; sendMessage(Map.of("type", "SETUP", "channel", 0, "version", "0.1-js/1.0.0", "keepaliveTimeout", 60, "acceptKeepaliveTimeout", 60)); }
        @Override protected void handleTextMessage(WebSocketSession s, TextMessage m) { processRawMessage(m.getPayload()); }
        @Override public void afterConnectionClosed(WebSocketSession s, CloseStatus st) { authenticated = false; channels.clear(); if (st.getCode() != CloseStatus.NORMAL.getCode()) scheduleReconnect(); }
    }
}
