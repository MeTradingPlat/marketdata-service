package com.metradingplat.marketdata.adapter.websocket;

import com.fasterxml.jackson.databind.JsonNode;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;

/**
 * WebSocket handler for real-time Greeks streaming.
 * 
 * Endpoint: /ws/greeks
 * 
 * Handles:
 * - Subscribe to Greeks for specific option symbols
 * - Unsubscribe from Greeks
 * - Forward Greeks updates to subscribed clients
 */
@Slf4j
public class GreeksWebSocketHandler extends BaseWebSocketHandler {

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
                log.info("Client {} subscribed to Greeks for symbol: {}", session.getId(), symbol);
            } else if ("unsubscribe".equals(action)) {
                unsubscribe(session, symbol);
                sendToSession(session, objectMapper.writeValueAsString(
                    objectMapper.createObjectNode()
                        .put("type", "subscription_cancelled")
                        .put("symbol", symbol)
                        .put("action", "unsubscribe")
                ));
                log.info("Client {} unsubscribed from Greeks for symbol: {}", session.getId(), symbol);
            }
        } catch (Exception e) {
            log.error("Error handling Greeks message", e);
            sendToSession(session, objectMapper.writeValueAsString(
                objectMapper.createObjectNode()
                    .put("type", "error")
                    .put("message", e.getMessage())
            ));
        }
    }

    @Override
    protected void onFirstSubscriberAdded(String symbol) {
        log.info("First subscriber added for Greeks: {}", symbol);
        // TODO: Subscribe to dxLink for this symbol
    }

    @Override
    protected void onLastSubscriberRemoved(String symbol) {
        log.info("Last subscriber removed for Greeks: {}", symbol);
        // TODO: Unsubscribe from dxLink for this symbol
    }
}
