package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.WebSocket;
import java.util.concurrent.CompletionStage;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Cliente para el Account Streamer de TastyTrade.
 * Escucha eventos de órdenes, posiciones y balances en tiempo real.
 */
@Slf4j
@Component
public class AccountStreamerClient implements WebSocket.Listener {

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final ScheduledExecutorService scheduler = Executors.newSingleThreadScheduledExecutor();
    private WebSocket webSocket;
    private String sessionToken;
    private String currentUrl;
    private volatile boolean authenticated = false;
    private final java.util.concurrent.atomic.AtomicInteger reconnectAttempts = new java.util.concurrent.atomic.AtomicInteger(0);
    private final AtomicBoolean connecting = new AtomicBoolean(false);
    private volatile ScheduledFuture<?> heartbeatTask;
    private OrderEventListener orderListener;

    public interface OrderEventListener {
        void onOrderEvent(String orderId, String symbol, String status, JsonNode fullData);
    }

    public void setOrderListener(OrderEventListener listener) {
        this.orderListener = listener;
    }

    // ensureConnected() (TastyTradeService) llama a connect() en CADA peticion
    // de velas, no solo cuando hace falta reconectar -- sin guarda, cada
    // llamada abria un WebSocket nuevo sin cerrar el anterior (quedaba
    // huerfano, nunca desconectado) y registraba otro heartbeat mas encima
    // del que ya corria, acumulando ambos sin limite mientras el proceso
    // siguiera vivo (confirmado en vivo: docenas de "Connecting to Account
    // Streamer" en minutos, uno por cada request de velas).
    public void connect(String url, String token) {
        this.sessionToken = token;
        this.currentUrl = url;
        if (webSocket != null && !webSocket.isOutputClosed()) return;
        if (!connecting.compareAndSet(false, true)) return;
        log.info("Connecting to Account Streamer: {}", url);

        try {
            HttpClient.newHttpClient().newWebSocketBuilder()
                    .buildAsync(URI.create(url), this)
                    .join();
            reconnectAttempts.set(0);
        } catch (Exception e) {
            log.error("Failed to connect to Account Streamer: {}", e.getMessage());
            scheduleReconnect();
        } finally {
            connecting.set(false);
        }
    }

    @Override
    public void onOpen(WebSocket webSocket) {
        this.webSocket = webSocket;
        log.info("Account Streamer connection opened.");

        // 1. Autenticación inmediata
        String authPayload = String.format("{\"action\": \"connect\", \"token\": \"%s\"}", sessionToken);
        webSocket.sendText(authPayload, true);

        // 2. Suscripción a eventos de cuenta
        // Nota: En una implementación real, se pueden pedir "watchwords" específicos
        String subPayload = "{\"action\": \"public-watchwords-subscribe\"}";
        webSocket.sendText(subPayload, true);

        // 3. Heartbeats cada 10 segundos -- cancelar el anterior antes de
        // programar uno nuevo, si no cada reconexion deja el viejo corriendo
        // para siempre junto al nuevo.
        if (heartbeatTask != null) heartbeatTask.cancel(false);
        heartbeatTask = scheduler.scheduleAtFixedRate(() -> {
            if (webSocket != null && !webSocket.isOutputClosed()) {
                webSocket.sendText("{\"action\": \"heartbeat\"}", true);
            }
        }, 10, 10, TimeUnit.SECONDS);

        webSocket.request(1);
    }

    @Override
    public CompletionStage<?> onText(WebSocket webSocket, CharSequence data, boolean last) {
        String message = data.toString();
        try {
            JsonNode root = objectMapper.readTree(message);
            String type = root.path("type").asText();
            
            if ("Order".equals(type)) {
                processOrderEvent(root.path("data"));
            } else if ("Connect".equals(type)) {
                log.info("Account Streamer authenticated successfully.");
                this.authenticated = true;
            } else if ("Heartbeat".equals(type)) {
                log.debug("Account Streamer heartbeat received.");
            }
            
        } catch (Exception e) {
            log.error("Error processing Account Streamer message: {}", message, e);
        }
        
        webSocket.request(1);
        return null;
    }

    private void processOrderEvent(JsonNode data) {
        String orderId = data.path("id").asText();
        String status = data.path("status").asText();
        String symbol = data.path("legs").path(0).path("symbol").asText();
        
        log.info("Order Event: {} | Symbol: {} | Status: {}", orderId, symbol, status);
        
        if (orderListener != null) {
            orderListener.onOrderEvent(orderId, symbol, status, data);
        }

        if ("Filled".equals(status)) {
            double remaining = data.path("remaining-quantity").asDouble();
            if (remaining == 0) {
                log.info("⚡ [FULL FILL] Order {} for {} fully executed.", orderId, symbol);
            } else {
                log.info("⏳ [PARTIAL FILL] Order {} for {} has {} remaining.", orderId, symbol, remaining);
            }
        } else if ("Rejected".equals(status)) {
            log.error("❌ [REJECTED] Order {} rejected. Reason: {}", orderId, data.path("reject-reason").asText());
        }
    }

    @Override
    public CompletionStage<?> onClose(WebSocket webSocket, int statusCode, String reason) {
        log.warn("Account Streamer connection closed: {} - {}", statusCode, reason);
        this.authenticated = false;
        scheduleReconnect();
        return null;
    }

    private void scheduleReconnect() {
        int delay = Math.min(30, (int) Math.pow(2, reconnectAttempts.getAndIncrement()));
        log.info("Scheduling Account Streamer reconnection in {} seconds...", delay);
        scheduler.schedule(() -> connect(currentUrl, sessionToken), delay, TimeUnit.SECONDS);
    }

    @Override
    public void onError(WebSocket webSocket, Throwable error) {
        log.error("Account Streamer error", error);
    }
}
