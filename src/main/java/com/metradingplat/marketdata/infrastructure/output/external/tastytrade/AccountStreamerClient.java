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
import java.util.concurrent.TimeUnit;

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
    private boolean authenticated = false;

    public void connect(String url, String token) {
        this.sessionToken = token;
        log.info("Connecting to Account Streamer: {}", url);
        
        HttpClient.newHttpClient().newWebSocketBuilder()
                .buildAsync(URI.create(url), this)
                .join();
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

        // 3. Heartbeats cada 10 segundos
        scheduler.scheduleAtFixedRate(() -> {
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
        
        if ("Filled".equals(status)) {
            double remaining = data.path("remaining-quantity").asDouble();
            if (remaining == 0) {
                log.info("⚡ [FULL FILL] Order {} for {} fully executed.", orderId, symbol);
                // TODO: Notificar al resto del sistema vía EventBus o Callback
            } else {
                log.info("⏳ [PARTIAL FILL] Order {} for {} has {} remaining.", orderId, symbol, remaining);
            }
        } else if ("Rejected".equals(status)) {
            log.error("❌ [REJECTED] Order {} rejected. Reason: {}", orderId, data.path("reject-reason").asText());
        }
    }

    @Override
    public void onClose(WebSocket webSocket, int statusCode, String reason) {
        log.warn("Account Streamer connection closed: {} - {}", statusCode, reason);
        this.authenticated = false;
        // TODO: Implementar reconexión automática
    }

    @Override
    public void onError(WebSocket webSocket, Throwable error) {
        log.error("Account Streamer error", error);
    }
}
