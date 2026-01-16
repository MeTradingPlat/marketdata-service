package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.net.URI;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.function.BiConsumer;

import org.springframework.stereotype.Component;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.TextWebSocketHandler;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.kafka.DTO.MarketDataStreamDTO;

import jakarta.annotation.PreDestroy;
import lombok.extern.slf4j.Slf4j;

/**
 * Cliente WebSocket para DxLink (streaming de datos de mercado).
 */
@Component
@Slf4j
public class DxLinkClient {

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final Set<String> subscribedSymbols = ConcurrentHashMap.newKeySet();
    private final ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();

    private WebSocketSession session;
    private String apiQuoteToken;
    private int channelId = 1;
    private boolean authenticated = false;

    private BiConsumer<String, MarketDataStreamDTO> onMarketData;
    private BiConsumer<String, Candle> onCandle;

    public void setOnMarketData(BiConsumer<String, MarketDataStreamDTO> callback) {
        this.onMarketData = callback;
    }

    public void setOnCandle(BiConsumer<String, Candle> callback) {
        this.onCandle = callback;
    }

    /**
     * Conecta al WebSocket DxLink.
     */
    public void connect(String url, String token) {
        this.apiQuoteToken = token;
        log.info("Connecting to DxLink: {}", url);

        try {
            StandardWebSocketClient client = new StandardWebSocketClient();
            this.session = client.execute(new DxLinkHandler(), url).get(10, TimeUnit.SECONDS);
            log.info("WebSocket connected");
        } catch (Exception e) {
            log.error("Failed to connect to DxLink", e);
            throw new RuntimeException("DxLink connection failed", e);
        }
    }

    /**
     * Suscribe a quotes y trades de un símbolo.
     */
    public void subscribe(String symbol) {
        if (!authenticated) {
            log.warn("Not authenticated yet, queuing subscription for: {}", symbol);
            subscribedSymbols.add(symbol);
            return;
        }

        if (subscribedSymbols.contains(symbol)) {
            log.debug("Already subscribed to: {}", symbol);
            return;
        }

        subscribedSymbols.add(symbol);
        sendSubscription(List.of(symbol), List.of("Quote", "Trade"));
        log.info("Subscribed to: {}", symbol);
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
                        Map.of("type", "Quote", "symbol", symbol),
                        Map.of("type", "Trade", "symbol", symbol)
                )
        ));
        log.info("Unsubscribed from: {}", symbol);
    }

    /**
     * Suscribe a candles históricos.
     */
    public void subscribeCandles(String symbol, String timeframe, long fromTime) {
        String candleSymbol = symbol + "{=" + timeframe + "}";
        sendMessage(Map.of(
                "type", "FEED_SUBSCRIPTION",
                "channel", channelId,
                "add", List.of(Map.of(
                        "type", "Candle",
                        "symbol", candleSymbol,
                        "fromTime", fromTime
                ))
        ));
        log.info("Subscribed to candles: {} from {}", candleSymbol, Instant.ofEpochMilli(fromTime));
    }

    public void disconnect() {
        try {
            if (session != null && session.isOpen()) {
                session.close();
            }
        } catch (Exception e) {
            log.error("Error closing WebSocket", e);
        }
        scheduler.shutdown();
    }

    public boolean isConnected() {
        return session != null && session.isOpen() && authenticated;
    }

    @PreDestroy
    public void cleanup() {
        disconnect();
    }

    private void sendMessage(Object message) {
        try {
            String json = objectMapper.writeValueAsString(message);
            log.debug("Sending: {}", json);
            session.sendMessage(new TextMessage(json));
        } catch (Exception e) {
            log.error("Failed to send message", e);
        }
    }

    private void sendSubscription(List<String> symbols, List<String> eventTypes) {
        var subscriptions = symbols.stream()
                .flatMap(symbol -> eventTypes.stream()
                        .map(type -> Map.of("type", type, "symbol", symbol)))
                .toList();

        sendMessage(Map.of(
                "type", "FEED_SUBSCRIPTION",
                "channel", channelId,
                "add", subscriptions
        ));
    }

    private void handleMessage(String payload) {
        try {
            JsonNode msg = objectMapper.readTree(payload);
            String type = msg.path("type").asText();

            switch (type) {
                case "SETUP" -> handleSetup(msg);
                case "AUTH_STATE" -> handleAuthState(msg);
                case "CHANNEL_OPENED" -> handleChannelOpened(msg);
                case "FEED_DATA" -> handleFeedData(msg);
                case "KEEPALIVE" -> sendMessage(Map.of("type", "KEEPALIVE", "channel", 0));
                default -> log.debug("Received: {}", type);
            }
        } catch (Exception e) {
            log.error("Error handling message: {}", payload, e);
        }
    }

    private void handleSetup(JsonNode msg) {
        log.info("Received SETUP, sending response");
        sendMessage(Map.of(
                "type", "SETUP",
                "channel", 0,
                "version", "0.1-DXF-JS/0.3.0",
                "keepaliveTimeout", 60,
                "acceptKeepaliveTimeout", 60
        ));

        // Enviar AUTH
        sendMessage(Map.of(
                "type", "AUTH",
                "channel", 0,
                "token", apiQuoteToken
        ));
    }

    private void handleAuthState(JsonNode msg) {
        String state = msg.path("state").asText();
        if ("AUTHORIZED".equals(state)) {
            log.info("DxLink authenticated successfully");
            authenticated = true;

            // Abrir canal para FEED
            sendMessage(Map.of(
                    "type", "CHANNEL_REQUEST",
                    "channel", channelId,
                    "service", "FEED",
                    "parameters", Map.of("contract", "AUTO")
            ));
        } else {
            log.error("Authentication failed: {}", state);
        }
    }

    private void handleChannelOpened(JsonNode msg) {
        log.info("Feed channel opened");

        // Configurar feed
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

        // Suscribir símbolos pendientes
        if (!subscribedSymbols.isEmpty()) {
            sendSubscription(List.copyOf(subscribedSymbols), List.of("Quote", "Trade"));
        }

        // Iniciar keepalive
        scheduler.scheduleAtFixedRate(
                () -> sendMessage(Map.of("type", "KEEPALIVE", "channel", 0)),
                30, 30, TimeUnit.SECONDS
        );
    }

    private void handleFeedData(JsonNode msg) {
        JsonNode data = msg.path("data");
        if (!data.isArray()) return;

        for (int i = 0; i < data.size(); i++) {
            JsonNode item = data.get(i);
            if (item.isTextual()) {
                // Es el tipo de evento (Quote, Trade, Candle)
                String eventType = item.asText();
                if (i + 1 < data.size()) {
                    JsonNode eventData = data.get(++i);
                    processEvent(eventType, eventData);
                }
            }
        }
    }

    private void processEvent(String eventType, JsonNode data) {
        try {
            String symbol = data.path(0).asText(); // eventSymbol es el primer campo

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
                        // Extraer símbolo base (remover {=timeframe})
                        String baseSymbol = symbol.contains("{") ? symbol.substring(0, symbol.indexOf("{")) : symbol;
                        Candle candle = Candle.builder()
                                .symbol(baseSymbol)
                                .timestamp(Instant.ofEpochMilli(data.path(1).asLong()))
                                .open(data.path(2).asDouble())
                                .high(data.path(3).asDouble())
                                .low(data.path(4).asDouble())
                                .close(data.path(5).asDouble())
                                .volume(data.path(6).asDouble())
                                .build();
                        onCandle.accept(baseSymbol, candle);
                    }
                }
            }
        } catch (Exception e) {
            log.error("Error processing {} event", eventType, e);
        }
    }

    private class DxLinkHandler extends TextWebSocketHandler {
        @Override
        public void afterConnectionEstablished(WebSocketSession session) {
            log.info("WebSocket connection established");
        }

        @Override
        protected void handleTextMessage(WebSocketSession session, TextMessage message) {
            DxLinkClient.this.handleMessage(message.getPayload());
        }

        @Override
        public void handleTransportError(WebSocketSession session, Throwable exception) {
            log.error("WebSocket transport error", exception);
        }

        @Override
        public void afterConnectionClosed(WebSocketSession session, CloseStatus status) {
            log.warn("WebSocket closed: {}", status);
            authenticated = false;
        }
    }
}
