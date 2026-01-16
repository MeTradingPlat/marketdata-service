package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.util.Map;

import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import com.metradingplat.marketdata.domain.models.OrderRequest;
import com.metradingplat.marketdata.domain.models.OrderResponse;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Cliente REST para TastyTrade API.
 * Maneja autenticación OAuth 2.0 y envío de órdenes.
 */
@Component
@RequiredArgsConstructor
@Slf4j
public class TastyTradeClient {

    private final RestClient tastyTradeRestClient;
    private final TastyTradeConfig config;

    private volatile String accessToken;
    private volatile String apiQuoteToken;
    private volatile String dxlinkUrl;

    /**
     * Obtiene access token usando OAuth refresh_token flow.
     */
    public synchronized String refreshAccessToken() {
        log.info("Refreshing OAuth access token");

        Map<String, String> request = Map.of(
                "grant_type", "refresh_token",
                "refresh_token", config.getRefreshToken(),
                "client_id", config.getClientId(),
                "client_secret", config.getClientSecret()
        );

        @SuppressWarnings("unchecked")
        Map<String, Object> response = tastyTradeRestClient
                .post()
                .uri("/oauth/token")
                .contentType(MediaType.APPLICATION_JSON)
                .body(request)
                .retrieve()
                .body(Map.class);

        if (response == null || !response.containsKey("access_token")) {
            throw new RuntimeException("Failed to get access token from TastyTrade");
        }

        this.accessToken = (String) response.get("access_token");
        log.info("Access token obtained, expires in {} seconds", response.get("expires_in"));
        return this.accessToken;
    }

    /**
     * Obtiene API quote token para DxLink WebSocket.
     */
    public synchronized String refreshApiQuoteToken() {
        log.info("Refreshing API quote token");

        if (accessToken == null) {
            refreshAccessToken();
        }

        @SuppressWarnings("unchecked")
        Map<String, Object> response = tastyTradeRestClient
                .get()
                .uri("/api-quote-tokens")
                .header("Authorization", "Bearer " + accessToken)
                .retrieve()
                .body(Map.class);

        if (response == null || !response.containsKey("data")) {
            throw new RuntimeException("Failed to get API quote token");
        }

        @SuppressWarnings("unchecked")
        Map<String, Object> data = (Map<String, Object>) response.get("data");
        this.apiQuoteToken = (String) data.get("token");
        this.dxlinkUrl = (String) data.get("dxlink-url");

        log.info("API quote token obtained, DxLink URL: {}", dxlinkUrl);
        return this.apiQuoteToken;
    }

    /**
     * Envía una orden a TastyTrade.
     */
    public OrderResponse submitOrder(OrderRequest order) {
        log.info("Submitting order: {} {} {} @ {}",
                order.getAction(), order.getQuantity(), order.getSymbol(), order.getPrice());

        if (accessToken == null) {
            refreshAccessToken();
        }

        // Construir el cuerpo de la orden según TastyTrade API
        Map<String, Object> leg = Map.of(
                "instrument-type", "Equity",
                "symbol", order.getSymbol(),
                "quantity", order.getQuantity(),
                "action", order.getAction().name()
        );

        Map<String, Object> orderBody = Map.of(
                "time-in-force", "Day",
                "order-type", order.getType().name(),
                "price", order.getPrice() != null ? order.getPrice().toString() : "0",
                "legs", new Map[]{leg}
        );

        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> response = tastyTradeRestClient
                    .post()
                    .uri("/accounts/{accountNumber}/orders", config.getAccountNumber())
                    .header("Authorization", "Bearer " + accessToken)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(orderBody)
                    .retrieve()
                    .body(Map.class);

            if (response == null) {
                throw new RuntimeException("Empty response from order submission");
            }

            @SuppressWarnings("unchecked")
            Map<String, Object> data = (Map<String, Object>) response.get("data");
            @SuppressWarnings("unchecked")
            Map<String, Object> orderData = (Map<String, Object>) data.get("order");

            return OrderResponse.builder()
                    .orderId(String.valueOf(orderData.get("id")))
                    .status((String) orderData.get("status"))
                    .build();

        } catch (Exception e) {
            log.error("Order submission failed", e);
            // Si es 401, renovar token y reintentar
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                log.info("Token expired, refreshing and retrying");
                refreshAccessToken();
                return submitOrder(order);
            }
            throw new RuntimeException("Order submission failed: " + e.getMessage(), e);
        }
    }

    public String getApiQuoteToken() {
        if (apiQuoteToken == null) {
            refreshApiQuoteToken();
        }
        return apiQuoteToken;
    }

    public String getDxlinkUrl() {
        if (dxlinkUrl == null) {
            refreshApiQuoteToken();
        }
        return dxlinkUrl != null ? dxlinkUrl : config.getDxlinkUrl();
    }

    public String getAccessToken() {
        if (accessToken == null) {
            refreshAccessToken();
        }
        return accessToken;
    }
}
