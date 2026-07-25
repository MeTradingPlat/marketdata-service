package com.metradingplat.marketdata.adapter.websocket;

import com.fasterxml.jackson.databind.JsonNode;
import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Consumer;

/**
 * WebSocket handler for real-time candle streaming.
 *
 * Endpoint: /ws/candles
 *
 * Protocol: client sends {"action":"subscribe","symbol":"AAPL","timeframe":"M1"};
 * server replies with one "history" message (recent closed bars) followed by "bar"
 * messages per update, each carrying closed:true/false (mirrors Binance kline "x")
 * so the client knows whether to replace or append the last point. The last
 * subscriber leaving a symbol:timeframe pair tears down its DxLink live listener.
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class CandleWebSocketHandler extends BaseWebSocketHandler {

    private static final int HISTORY_BARS = 300;

    private final GestionarHistoricalDataCUIntPort historicalDataUseCase;
    private final GestionarComunicacionExternalGatewayIntPort externalGateway;
    private final Map<String, Consumer<Candle>> activeListeners = new ConcurrentHashMap<>();

    @Override
    protected void handleTextMessage(WebSocketSession session, TextMessage message) throws Exception {
        try {
            JsonNode node = objectMapper.readTree(message.getPayload());
            String action = node.get("action").asText();
            String symbol = node.get("symbol").asText().toUpperCase();
            EnumTimeframe timeframe = EnumTimeframe.valueOf(node.path("timeframe").asText("M1").toUpperCase());
            String subscriptionKey = symbol + ":" + timeframe.name();

            if ("subscribe".equals(action)) {
                // History is sent BEFORE registering the live listener: subscribe() triggers
                // onFirstSubscriberAdded synchronously, and once that listener is registered
                // any DxLink tick can broadcast to this session immediately. Doing it in the
                // other order let a live "bar" race ahead of "history" on the wire, causing
                // the client to update() a series that hadn't been setData()'d yet.
                sendHistory(session, symbol, timeframe);
                subscribe(session, subscriptionKey);
                confirm(session, symbol, timeframe, "subscribe", "subscription_confirmed");
                log.info("Client {} subscribed to candles {}", session.getId(), subscriptionKey);
            } else if ("unsubscribe".equals(action)) {
                unsubscribe(session, subscriptionKey);
                confirm(session, symbol, timeframe, "unsubscribe", "subscription_cancelled");
                log.info("Client {} unsubscribed from candles {}", session.getId(), subscriptionKey);
            }
        } catch (Exception e) {
            log.error("Error handling candle message", e);
            sendToSession(session, objectMapper.writeValueAsString(
                    objectMapper.createObjectNode()
                            .put("type", "error")
                            .put("message", e.getMessage())));
        }
    }

    @Override
    protected void onFirstSubscriberAdded(String subscriptionKey) {
        String symbol = symbolOf(subscriptionKey);
        EnumTimeframe timeframe = timeframeOf(subscriptionKey);
        log.info("First subscriber added for candles: {}", subscriptionKey);

        Consumer<Candle> listener = candle -> {
            if (!isComplete(candle)) return;
            boolean closed = !candle.getTimestamp().plus(timeframe.getDuration()).isAfter(Instant.now());
            broadcastToSubscribers(subscriptionKey,
                    CandleMessageBuilder.bar(objectMapper, symbol, timeframe.name(), candle, closed));
        };
        activeListeners.put(subscriptionKey, listener);
        externalGateway.addCandleLiveListener(symbol, timeframe, listener);
    }

    @Override
    protected void onLastSubscriberRemoved(String subscriptionKey) {
        log.info("Last subscriber removed for candles: {}", subscriptionKey);
        Consumer<Candle> listener = activeListeners.remove(subscriptionKey);
        if (listener != null) {
            externalGateway.removeCandleLiveListener(symbolOf(subscriptionKey), timeframeOf(subscriptionKey), listener);
        }
    }

    private void sendHistory(WebSocketSession session, String symbol, EnumTimeframe timeframe) throws Exception {
        List<Candle> bars = historicalDataUseCase.getCandles(symbol, timeframe, null, HISTORY_BARS).stream()
                .filter(CandleWebSocketHandler::isComplete)
                .toList();
        sendToSession(session, CandleMessageBuilder.history(objectMapper, symbol, timeframe.name(), bars));

        Candle current = historicalDataUseCase.getCurrentCandle(symbol, timeframe);
        if (current != null && isComplete(current)) {
            sendToSession(session, CandleMessageBuilder.bar(objectMapper, symbol, timeframe.name(), current, false));
        }
    }

    private static boolean isComplete(Candle candle) {
        return candle.getOpen() != null && candle.getHigh() != null
                && candle.getLow() != null && candle.getClose() != null;
    }

    private void confirm(WebSocketSession session, String symbol, EnumTimeframe timeframe, String action, String type)
            throws Exception {
        sendToSession(session, objectMapper.writeValueAsString(
                objectMapper.createObjectNode()
                        .put("type", type)
                        .put("symbol", symbol)
                        .put("timeframe", timeframe.name())
                        .put("action", action)));
    }

    private static String symbolOf(String subscriptionKey) {
        return subscriptionKey.substring(0, subscriptionKey.indexOf(':'));
    }

    private static EnumTimeframe timeframeOf(String subscriptionKey) {
        return EnumTimeframe.valueOf(subscriptionKey.substring(subscriptionKey.indexOf(':') + 1));
    }
}
