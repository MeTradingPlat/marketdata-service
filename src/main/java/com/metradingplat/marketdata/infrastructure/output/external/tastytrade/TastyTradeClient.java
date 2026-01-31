package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.BracketOrder;
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

        // Guardar el nuevo refresh_token para que no expire
        if (response.containsKey("refresh_token")) {
            String newRefreshToken = (String) response.get("refresh_token");
            config.setRefreshToken(newRefreshToken);
            log.info("Refresh token updated successfully");
        }

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
        Map<String, Object> response;
        try {
            response = tastyTradeRestClient
                    .get()
                    .uri("/api-quote-tokens")
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(Map.class);
        } catch (Exception e) {
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                log.info("Access token expired while getting quote token, refreshing...");
                refreshAccessToken();
                response = tastyTradeRestClient
                        .get()
                        .uri("/api-quote-tokens")
                        .header("Authorization", "Bearer " + accessToken)
                        .retrieve()
                        .body(Map.class);
            } else {
                throw e;
            }
        }

        log.info("API quote token response: {}", response);

        if (response == null || !response.containsKey("data")) {
            throw new RuntimeException("Failed to get API quote token");
        }

        @SuppressWarnings("unchecked")
        Map<String, Object> data = (Map<String, Object>) response.get("data");
        this.apiQuoteToken = (String) data.get("token");
        this.dxlinkUrl = (String) data.get("dxlink-url");

        log.info("API quote token obtained (length={}), DxLink URL: {}",
            apiQuoteToken != null ? apiQuoteToken.length() : 0, dxlinkUrl);
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
        // Siempre obtener un token fresco para evitar problemas de expiración
        refreshApiQuoteToken();
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

    /**
     * Obtiene equities activos (paginado) desde TastyTrade.
     * GET /instruments/equities/active?per-page={perPage}&page-offset={pageOffset}
     */
    @SuppressWarnings("unchecked")
    public List<ActiveEquity> getActiveEquities(int pageOffset, int perPage) {
        if (accessToken == null) refreshAccessToken();

        try {
            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri(uriBuilder -> uriBuilder
                            .path("/instruments/equities/active")
                            .queryParam("per-page", perPage)
                            .queryParam("page-offset", pageOffset)
                            .build())
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(Map.class);

            if (response == null || !response.containsKey("data")) {
                return List.of();
            }

            Map<String, Object> data = (Map<String, Object>) response.get("data");
            List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
            if (items == null) return List.of();

            List<ActiveEquity> equities = new ArrayList<>();
            for (Map<String, Object> item : items) {
                equities.add(ActiveEquity.builder()
                        .symbol((String) item.get("symbol"))
                        .description((String) item.get("description"))
                        .listedMarket((String) item.get("listed-market"))
                        .build());
            }
            return equities;

        } catch (Exception e) {
            log.error("Failed to get active equities: {}", e.getMessage());
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getActiveEquities(pageOffset, perPage);
            }
            return List.of();
        }
    }

    /**
     * Obtiene quote actual via TastyTrade market data.
     * GET /market-data/by-type?equity={symbol}
     */
    @SuppressWarnings("unchecked")
    public Map<String, Object> getMarketDataByType(String symbol) {
        if (accessToken == null) refreshAccessToken();

        try {
            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri(uriBuilder -> uriBuilder
                            .path("/market-data/by-type")
                            .queryParam("equity", symbol)
                            .build())
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(Map.class);

            if (response == null || !response.containsKey("data")) {
                return Map.of();
            }
            return (Map<String, Object>) response.get("data");

        } catch (Exception e) {
            log.error("Failed to get market data for {}: {}", symbol, e.getMessage());
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getMarketDataByType(symbol);
            }
            return Map.of();
        }
    }

    /**
     * Obtiene earnings reports historicos.
     * GET /market-metrics/historic-corporate-events/earnings-reports/{symbol}?start-date={startDate}
     */
    @SuppressWarnings("unchecked")
    public List<Map<String, Object>> getEarningsReports(String symbol, String startDate) {
        if (accessToken == null) refreshAccessToken();

        try {
            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri(uriBuilder -> uriBuilder
                            .path("/market-metrics/historic-corporate-events/earnings-reports/{symbol}")
                            .queryParam("start-date", startDate)
                            .build(symbol))
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(Map.class);

            if (response == null || !response.containsKey("data")) {
                return List.of();
            }

            Map<String, Object> data = (Map<String, Object>) response.get("data");
            List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
            return items != null ? items : List.of();

        } catch (Exception e) {
            log.error("Failed to get earnings for {}: {}", symbol, e.getMessage());
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getEarningsReports(symbol, startDate);
            }
            return List.of();
        }
    }

    /**
     * Envía una orden bracket (OTOCO) a TastyTrade.
     * POST /accounts/{accountNumber}/orders
     */
    public OrderResponse submitBracketOrder(BracketOrder order) {
        log.info("Submitting bracket order: {} {} {} @ entry={} SL={} TP={}",
                order.getAction(), order.getQuantity(), order.getSymbol(),
                order.getEntryPrice(), order.getStopLossPrice(), order.getTakeProfitPrice());

        if (accessToken == null) refreshAccessToken();

        // Accion de cierre inversa
        String closeAction = order.getAction().name().contains("BUY")
                ? "Sell to Close" : "Buy to Close";

        // Leg de entrada
        Map<String, Object> entryLeg = Map.of(
                "instrument-type", "Equity",
                "symbol", order.getSymbol(),
                "quantity", order.getQuantity(),
                "action", order.getAction().name().replace("_", " ")
        );

        Map<String, Object> entryOrder = Map.of(
                "time-in-force", order.getTimeInForce() != null ? order.getTimeInForce() : "Day",
                "order-type", "Limit",
                "price", order.getEntryPrice().toString(),
                "legs", new Map[]{entryLeg}
        );

        // Leg de take profit (LIMIT)
        Map<String, Object> tpLeg = Map.of(
                "instrument-type", "Equity",
                "symbol", order.getSymbol(),
                "quantity", order.getQuantity(),
                "action", closeAction
        );

        Map<String, Object> tpOrder = Map.of(
                "time-in-force", "GTC",
                "order-type", "Limit",
                "price", order.getTakeProfitPrice().toString(),
                "legs", new Map[]{tpLeg}
        );

        // Leg de stop loss (STOP)
        Map<String, Object> slLeg = Map.of(
                "instrument-type", "Equity",
                "symbol", order.getSymbol(),
                "quantity", order.getQuantity(),
                "action", closeAction
        );

        Map<String, Object> slOrder = Map.of(
                "time-in-force", "GTC",
                "order-type", "Stop",
                "stop-trigger", order.getStopLossPrice().toString(),
                "legs", new Map[]{slLeg}
        );

        // OTOCO: One Triggers One Cancels Other
        Map<String, Object> otoco = Map.of(
                "type", "OTOCO",
                "trigger-order", entryOrder,
                "orders", new Map[]{tpOrder, slOrder}
        );

        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> response = tastyTradeRestClient
                    .post()
                    .uri("/accounts/{accountNumber}/complex-orders", config.getAccountNumber())
                    .header("Authorization", "Bearer " + accessToken)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(otoco)
                    .retrieve()
                    .body(Map.class);

            if (response == null) {
                throw new RuntimeException("Empty response from bracket order submission");
            }

            @SuppressWarnings("unchecked")
            Map<String, Object> data = (Map<String, Object>) response.get("data");

            return OrderResponse.builder()
                    .orderId(String.valueOf(data.get("id")))
                    .status("RECEIVED")
                    .complexOrderId(String.valueOf(data.get("id")))
                    .build();

        } catch (Exception e) {
            log.error("Bracket order submission failed", e);
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return submitBracketOrder(order);
            }
            throw new RuntimeException("Bracket order failed: " + e.getMessage(), e);
        }
    }

    /**
     * Cancela una orden.
     * DELETE /accounts/{accountNumber}/orders/{orderId}
     */
    public void cancelOrder(String orderId) {
        log.info("Cancelling order: {}", orderId);

        if (accessToken == null) refreshAccessToken();

        try {
            tastyTradeRestClient
                    .delete()
                    .uri("/accounts/{accountNumber}/orders/{orderId}",
                            config.getAccountNumber(), orderId)
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .toBodilessEntity();

            log.info("Order {} cancelled successfully", orderId);

        } catch (Exception e) {
            log.error("Failed to cancel order {}: {}", orderId, e.getMessage());
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                cancelOrder(orderId);
                return;
            }
            throw new RuntimeException("Cancel order failed: " + e.getMessage(), e);
        }
    }
}
