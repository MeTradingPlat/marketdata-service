package com.metradingplat.marketdata.adapter.websocket;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.PingMessage;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.handler.TextWebSocketHandler;

import java.io.IOException;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;

/**
 * Base WebSocket handler for market data streaming.
 *
 * Provides:
 * - Subscription management (track subscribed clients)
 * - Message routing to subscribed clients
 * - Connection lifecycle management
 * - Error handling and logging
 */
@Slf4j
public abstract class BaseWebSocketHandler extends TextWebSocketHandler {

    protected final ObjectMapper objectMapper = new ObjectMapper();
    protected final Map<String, Set<WebSocketSession>> subscriptions = new ConcurrentHashMap<>();
    protected final Map<WebSocketSession, Set<String>> sessionSubscriptions = new ConcurrentHashMap<>();

    // Gateway's Netty server.netty.idle-timeout closes any connection with no
    // byte-level activity for 300s. A candle subscription can otherwise sit
    // silent for a long time (market closed, no live ticks), so ping every
    // 30s to keep it alive -- same interval the SSE service already uses for
    // the same reason. A native WS ping is answered by the browser
    // automatically, no client-side handling needed.
    private static final int HEARTBEAT_SECONDS = 30;
    private final ScheduledExecutorService heartbeatScheduler = Executors.newSingleThreadScheduledExecutor();

    @PostConstruct
    private void startHeartbeat() {
        heartbeatScheduler.scheduleAtFixedRate(this::sendHeartbeat, HEARTBEAT_SECONDS, HEARTBEAT_SECONDS, TimeUnit.SECONDS);
    }

    @PreDestroy
    private void stopHeartbeat() {
        heartbeatScheduler.shutdownNow();
    }

    private void sendHeartbeat() {
        for (WebSocketSession session : sessionSubscriptions.keySet()) {
            try {
                if (session.isOpen()) {
                    session.sendMessage(new PingMessage());
                }
            } catch (IOException e) {
                log.warn("Failed to send heartbeat ping to session {}: {}", session.getId(), e.getMessage());
            }
        }
    }

    /**
     * Handle new WebSocket connection.
     */
    @Override
    public void afterConnectionEstablished(WebSocketSession session) throws Exception {
        log.info("WebSocket connection established: {}", session.getId());
        sessionSubscriptions.put(session, ConcurrentHashMap.newKeySet());
    }

    /**
     * Handle WebSocket disconnection.
     */
    @Override
    public void afterConnectionClosed(WebSocketSession session, CloseStatus status) throws Exception {
        log.info("WebSocket connection closed: {} (status: {})", session.getId(), status);
        
        // Remove all subscriptions for this session
        Set<String> symbols = sessionSubscriptions.remove(session);
        if (symbols != null) {
            for (String symbol : symbols) {
                Set<WebSocketSession> sessions = subscriptions.get(symbol);
                if (sessions != null) {
                    sessions.remove(session);
                    if (sessions.isEmpty()) {
                        subscriptions.remove(symbol);
                        onLastSubscriberRemoved(symbol);
                    }
                }
            }
        }
    }

    /**
     * Subscribe session to symbol.
     */
    protected void subscribe(WebSocketSession session, String symbol) {
        log.debug("Subscribing session {} to symbol: {}", session.getId(), symbol);

        Set<WebSocketSession> sessions = subscriptions.computeIfAbsent(symbol, k -> ConcurrentHashMap.newKeySet());
        boolean wasEmpty = sessions.isEmpty();
        sessions.add(session);
        sessionSubscriptions.computeIfAbsent(session, k -> ConcurrentHashMap.newKeySet()).add(symbol);

        if (wasEmpty) {
            onFirstSubscriberAdded(symbol);
        }
    }

    /**
     * Unsubscribe session from symbol.
     */
    protected void unsubscribe(WebSocketSession session, String symbol) {
        log.debug("Unsubscribing session {} from symbol: {}", session.getId(), symbol);
        
        Set<WebSocketSession> sessions = subscriptions.get(symbol);
        if (sessions != null) {
            sessions.remove(session);
            if (sessions.isEmpty()) {
                subscriptions.remove(symbol);
                onLastSubscriberRemoved(symbol);
            }
        }
        
        Set<String> symbols = sessionSubscriptions.get(session);
        if (symbols != null) {
            symbols.remove(symbol);
        }
    }

    /**
     * Broadcast message to all subscribers of a symbol.
     */
    protected void broadcastToSubscribers(String symbol, String message) {
        Set<WebSocketSession> sessions = subscriptions.get(symbol);
        if (sessions != null && !sessions.isEmpty()) {
            for (WebSocketSession session : sessions) {
                try {
                    if (session.isOpen()) {
                        session.sendMessage(new TextMessage(message));
                    }
                } catch (IOException e) {
                    log.error("Error sending message to session {}", session.getId(), e);
                }
            }
        }
    }

    /**
     * Send message to specific session.
     */
    protected void sendToSession(WebSocketSession session, String message) throws IOException {
        if (session.isOpen()) {
            session.sendMessage(new TextMessage(message));
        }
    }

    /**
     * Called when first subscriber is added for a symbol.
     * Override to implement custom logic (e.g., subscribe to external data source).
     */
    protected void onFirstSubscriberAdded(String symbol) {
        log.debug("First subscriber added for symbol: {}", symbol);
    }

    /**
     * Called when last subscriber is removed for a symbol.
     * Override to implement custom logic (e.g., unsubscribe from external data source).
     */
    protected void onLastSubscriberRemoved(String symbol) {
        log.debug("Last subscriber removed for symbol: {}", symbol);
    }

    /**
     * Get number of active subscriptions.
     */
    public int getActiveSubscriptionCount() {
        return subscriptions.size();
    }

    /**
     * Get number of active sessions.
     */
    public int getActiveSessionCount() {
        return sessionSubscriptions.size();
    }
}
