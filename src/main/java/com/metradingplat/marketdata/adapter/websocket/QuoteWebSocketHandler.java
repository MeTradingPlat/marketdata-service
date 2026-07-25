package com.metradingplat.marketdata.adapter.websocket;

import com.fasterxml.jackson.databind.JsonNode;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;

/**
 * WebSocket handler for real-time quote streaming.
 * 
 * Endpoint: /ws/quotes
 * 
 * Handles:
 * - Subscribe to quotes for specific symbols
 * - Unsubscribe from quotes
 * - Forward quote updates to subscribed clients
 */
@Slf4j
public class QuoteWebSocketHandler extends BaseWebSocketHandler {

    @Override
    protected void handleTextMessage(WebSocketSession session, TextMessage message) throws Exception {
        try {
            JsonNode node = objectMapper.readTree(message.getPayload());
            String action = node.get("action").asText();
            String symbol = node.get("symbol").asText();

            if ("subscribe".equals(action)) {
                subscribe(session, symbol);
                sendToSession(session, objectMapper.writeValueAsString(
                    objectMapper.createObjectNode()
                        .put("type", "subscription_confirmed")
                        .put("symbol", symbol)
                        .put("action", "subscribe")
                ));
                log.info("Client {} subscribed to quotes for symbol: {}", session.getId(), symbol);
            } else if ("unsubscribe".equals(action)) {
                unsubscribe(session, symbol);
                sendToSession(session, objectMapper.writeValueAsString(
                    objectMapper.createObjectNode()
                        .put("type", "subscription_cancelled")
                        .put("symbol", symbol)
                        .put("action", "unsubscribe")
                ));
                log.info("Client {} unsubscribed from quotes for symbol: {}", session.getId(), symbol);
            }
        } catch (Exception e) {
            log.error("Error handling quote message", e);
            sendToSession(session, objectMapper.writeValueAsString(
                objectMapper.createObjectNode()
                    .put("type", "error")
                    .put("message", e.getMessage())
            ));
        }
    }

    @Override
    protected void onFirstSubscriberAdded(String symbol) {
        log.info("First subscriber added for quotes: {}", symbol);
        // TODO: Subscribe to dxLink for this symbol
    }

    @Override
    protected void onLastSubscriberRemoved(String symbol) {
        log.info("Last subscriber removed for quotes: {}", symbol);
        // TODO: Unsubscribe from dxLink for this symbol
    }
}
