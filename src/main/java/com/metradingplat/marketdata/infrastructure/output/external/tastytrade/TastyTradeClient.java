package com.metradingplat.marketdata.infrastructure.output.external.tastytrade;

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
import com.metradingplat.marketdata.domain.models.ComplexOrderModels;
import com.metradingplat.marketdata.domain.models.Candle;

import org.springframework.scheduling.annotation.Scheduled;
import java.util.concurrent.TimeUnit;
import java.time.Instant;
import java.time.ZoneOffset;

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

    private static final int MAX_RETRIES = 3;
    private static final long INITIAL_BACKOFF_MS = 2000;

    /**
     * Obtiene access token usando OAuth refresh_token flow.
     */
    public synchronized String refreshAccessToken() {
        log.info("Refreshing OAuth access token");

        Map<String, String> request = Map.of(
                "grant_type", "refresh_token",
                "refresh_token", config.getRefreshToken(),
                "client_id", config.getClientId(),
                "client_secret", config.getClientSecret());

        Map<String, Object> response = tastyTradeRestClient
                .post()
                .uri("/oauth/token")
                .contentType(MediaType.APPLICATION_JSON)
                .body(request)
                .retrieve()
                .body(new ParameterizedTypeReference<Map<String, Object>>() {
                });

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
     * Tarea programada para refrescar el token proactivamente cada 14 minutos.
     * Evita la degradacion de sesion (15 min) y los errores 401.
     */
    @Scheduled(fixedRate = 14, timeUnit = TimeUnit.MINUTES)
    public void proactiveTokenRefresh() {
        try {
            log.info("Proactive token refresh execution");
            refreshAccessToken();
            refreshApiQuoteToken();
        } catch (Exception e) {
            log.error("Failed proactive token refresh", e);
        }
    }

    /**
     * Obtiene API quote token para DxLink WebSocket.
     */
    public synchronized String refreshApiQuoteToken() {
        log.info("Refreshing API quote token");

        if (accessToken == null) {
            refreshAccessToken();
        }

        Map<String, Object> response;
        try {
            response = tastyTradeRestClient
                    .get()
                    .uri("/api-quote-tokens")
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(new ParameterizedTypeReference<Map<String, Object>>() {
                    });
        } catch (Exception e) {
            if (e.getMessage() != null && e.getMessage().contains("401")) {
                log.info("Access token expired while getting quote token, refreshing...");
                refreshAccessToken();
                response = tastyTradeRestClient
                        .get()
                        .uri("/api-quote-tokens")
                        .header("Authorization", "Bearer " + accessToken)
                        .retrieve()
                        .body(new ParameterizedTypeReference<Map<String, Object>>() {
                        });
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
        return submitOrderInternal(order, false);
    }

    private OrderResponse submitOrderInternal(OrderRequest order, boolean isRetry) {
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
                "action", order.getAction().name());

        Map<String, Object> orderBody = Map.of(
                "time-in-force", "Day",
                "order-type", order.getType().name(),
                "price", order.getPrice() != null ? order.getPrice().toString() : "0",
                "legs", new Map[] { leg });

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
            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                log.info("Token expired, refreshing and retrying");
                refreshAccessToken();
                return submitOrderInternal(order, true);
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
    public List<ActiveEquity> getActiveEquities(int pageOffset, int perPage) {
        return getActiveEquitiesInternal(pageOffset, perPage, false);
    }

    @SuppressWarnings("unchecked")
    private List<ActiveEquity> getActiveEquitiesInternal(int pageOffset, int perPage, boolean isRetry) {
        if (accessToken == null)
            refreshAccessToken();

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
            if (items == null)
                return List.of();

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
            
            // 429 Backoff Handling (Steering Rule)
            if (e.getMessage() != null && e.getMessage().contains("429")) {
                log.warn("HTTP 429 Too Many Requests in getActiveEquities. Retrying with backoff...");
                return executeWithBackoff(() -> getActiveEquitiesInternal(pageOffset, perPage, true), 1);
            }

            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getActiveEquitiesInternal(pageOffset, perPage, true);
            }
            return List.of();
        }
    }

    /**
     * Obtiene detalles técnicos de varios equities en una sola llamada.
     * Formato: ?symbol[]=AAPL&symbol[]=MSFT
     * Particionamiento: Chunks de 250 símbolos para evitar URLs excesivamente largas.
     */
    @SuppressWarnings("unchecked")
    public List<Map<String, Object>> getEquitiesBatch(List<String> symbols) {
        if (accessToken == null) refreshAccessToken();
        
        List<Map<String, Object>> allItems = new ArrayList<>();
        int chunkSize = 250;

        for (int i = 0; i < symbols.size(); i += chunkSize) {
            List<String> chunk = symbols.subList(i, Math.min(i + chunkSize, symbols.size()));
            try {
                Map<String, Object> response = tastyTradeRestClient
                        .get()
                        .uri(uriBuilder -> {
                            uriBuilder.path("/instruments/equities");
                            for (String s : chunk) {
                                uriBuilder.queryParam("symbol[]", s);
                            }
                            return uriBuilder.build();
                        })
                        .header("Authorization", "Bearer " + accessToken)
                        .retrieve()
                        .body(Map.class);
                
                if (response != null && response.containsKey("data")) {
                    Map<String, Object> data = (Map<String, Object>) response.get("data");
                    List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
                    if (items != null) allItems.addAll(items);
                }
            } catch (Exception e) {
                log.error("Failed to get equities batch chunk at index {}: {}", i, e.getMessage());
            }
        }
        return allItems;
    }

    /**
     * Obtiene cotizaciones en batch (REST).
     * Formato: ?equity=AAPL,MSFT
     * Particionamiento: Chunks de 100 símbolos (Límite rígido de la API para market-data).
     */
    @SuppressWarnings("unchecked")
    public List<Map<String, Object>> getMarketDataBatch(List<String> symbols) {
        if (accessToken == null) refreshAccessToken();
        
        List<Map<String, Object>> allItems = new ArrayList<>();
        int chunkSize = 100; // Límite estricto de TastyTrade para /market-data/by-type

        for (int i = 0; i < symbols.size(); i += chunkSize) {
            List<String> chunk = symbols.subList(i, Math.min(i + chunkSize, symbols.size()));
            String commaSeparated = String.join(",", chunk);
            try {
                Map<String, Object> response = tastyTradeRestClient
                        .get()
                        .uri(uriBuilder -> uriBuilder
                                .path("/market-data/by-type")
                                .queryParam("equity", commaSeparated)
                                .build())
                        .header("Authorization", "Bearer " + accessToken)
                        .retrieve()
                        .body(Map.class);
                
                if (response != null && response.containsKey("data")) {
                    Map<String, Object> data = (Map<String, Object>) response.get("data");
                    List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
                    if (items != null) allItems.addAll(items);
                }
            } catch (Exception e) {
                log.error("Failed to get market data batch chunk at index {}: {}", i, e.getMessage());
            }
        }
        return allItems;
    }

    /**
     * Obtiene quote actual via TastyTrade market data.
     * GET /market-data/by-type?equity={symbol}
     */
    public Map<String, Object> getMarketDataByType(String symbol) {
        return getMarketDataByTypeInternal(symbol, false);
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> getMarketDataByTypeInternal(String symbol, boolean isRetry) {
        if (accessToken == null)
            refreshAccessToken();

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
            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getMarketDataByTypeInternal(symbol, true);
            }
            return Map.of();
        }
    }

    /**
     * Obtiene earnings reports historicos.
     * GET
     * /market-metrics/historic-corporate-events/earnings-reports/{symbol}?start-date={startDate}
     */
    public List<Map<String, Object>> getEarningsReports(String symbol, String startDate) {
        return getEarningsReportsInternal(symbol, startDate, false);
    }

    @SuppressWarnings("unchecked")
    private List<Map<String, Object>> getEarningsReportsInternal(String symbol, String startDate, boolean isRetry) {
        if (accessToken == null)
            refreshAccessToken();

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
            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getEarningsReportsInternal(symbol, startDate, true);
            }
            return List.of();
        }
    }

    /**
     * Envía una orden bracket (OTOCO) a TastyTrade.
     * POST /accounts/{accountNumber}/orders
     */
    public OrderResponse submitBracketOrder(BracketOrder order) {
        return submitBracketOrderInternal(order, false, false);
    }

    public ComplexOrderModels.DryRunResponse validateBracketOrder(BracketOrder order) {
        return (ComplexOrderModels.DryRunResponse) submitBracketOrderInternal(order, false, true);
    }

    private Object submitBracketOrderInternal(BracketOrder order, boolean isRetry, boolean dryRun) {
        log.info("{} bracket order: {} {} {} @ entry={} SL={} TP={}",
                dryRun ? "Validating" : "Submitting",
                order.getAction(), order.getQuantity(), order.getSymbol(),
                order.getEntryPrice(), order.getStopLossPrice(), order.getTakeProfitPrice());

        if (accessToken == null) refreshAccessToken();

        String entryAction = order.getAction().name().replace("_", " ");
        String closeAction = order.getAction().name().contains("BUY") ? "Sell to Close" : "Buy to Close";
        
        // Determinar price-effect (Simplificado para Equity)
        String entryEffect = order.getAction().name().contains("BUY") ? "Debit" : "Credit";
        String closeEffect = order.getAction().name().contains("BUY") ? "Credit" : "Debit";

        // Leg de entrada
        var entryLeg = ComplexOrderModels.OrderLeg.builder()
                .instrumentType("Equity")
                .symbol(order.getSymbol())
                .quantity(order.getQuantity())
                .action(entryAction)
                .build();

        var triggerOrder = ComplexOrderModels.SingleOrder.builder()
                .timeInForce(order.getTimeInForce() != null ? order.getTimeInForce() : "Day")
                .orderType("Limit")
                .price(order.getEntryPrice().doubleValue())
                .priceEffect(entryEffect)
                .legs(List.of(entryLeg))
                .build();

        // Leg de salida (Reutilizable)
        var exitLeg = ComplexOrderModels.OrderLeg.builder()
                .instrumentType("Equity")
                .symbol(order.getSymbol())
                .quantity(order.getQuantity())
                .action(closeAction)
                .build();

        // Orden Take Profit (Limit)
        var tpOrder = ComplexOrderModels.SingleOrder.builder()
                .timeInForce("GTC")
                .orderType("Limit")
                .price(order.getTakeProfitPrice().doubleValue())
                .priceEffect(closeEffect)
                .legs(List.of(exitLeg))
                .build();

        // Orden Stop Loss (Stop)
        var slOrder = ComplexOrderModels.SingleOrder.builder()
                .timeInForce("GTC")
                .orderType("Stop")
                .stopTrigger(order.getStopLossPrice().doubleValue())
                .legs(List.of(exitLeg))
                .build();

        var otoco = ComplexOrderModels.ComplexOrderRequest.builder()
                .type("OTOCO")
                .triggerOrder(triggerOrder)
                .orders(List.of(tpOrder, slOrder))
                .build();

        String endpoint = "/accounts/{accountNumber}/complex-orders" + (dryRun ? "/dry-run" : "");

        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> response = tastyTradeRestClient
                    .post()
                    .uri(endpoint, config.getAccountNumber())
                    .header("Authorization", "Bearer " + accessToken)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(otoco)
                    .retrieve()
                    .body(Map.class);

            if (response == null) {
                throw new RuntimeException("Empty response from order " + (dryRun ? "validation" : "submission"));
            }

            @SuppressWarnings("unchecked")
            Map<String, Object> data = (Map<String, Object>) response.get("data");

            if (dryRun) {
                log.info("Bracket order validation successful");
                
                @SuppressWarnings("unchecked")
                Map<String, Object> bp = (Map<String, Object>) data.get("buying-power-effect");
                List<String> warnings = (List<String>) data.get("warnings");

                return ComplexOrderModels.DryRunResponse.builder()
                        .status("VALIDATED")
                        .warnings(warnings)
                        .buyingPowerEffect(bp != null ? ComplexOrderModels.BuyingPowerEffect.builder()
                                .changeInBuyingPower(safeConvertToDouble(bp.get("change-in-buying-power")))
                                .currentBuyingPower(safeConvertToDouble(bp.get("current-buying-power")))
                                .newBuyingPower(safeConvertToDouble(bp.get("new-buying-power")))
                                .buyingPowerEffectSign(safeConvertToInt(bp.get("buying-power-effect-sign")))
                                .build() : null)
                        .build();
            }

            return OrderResponse.builder()
                    .orderId(String.valueOf(data.get("id")))
                    .status("RECEIVED")
                    .complexOrderId(String.valueOf(data.get("id")))
                    .build();

        } catch (Exception e) {
            log.error("Bracket order {} failed", dryRun ? "validation" : "submission", e);
            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return submitBracketOrderInternal(order, true, dryRun);
            }
            throw new RuntimeException("Bracket order failed: " + e.getMessage(), e);
        }
    }

    /**
     * Cancela una orden.
     * DELETE /accounts/{accountNumber}/orders/{orderId}
     */
    public void cancelOrder(String orderId) {
        cancelOrderInternal(orderId, false);
    }

    private void cancelOrderInternal(String orderId, boolean isRetry) {
        log.info("Cancelling order: {}", orderId);

        if (accessToken == null)
            refreshAccessToken();

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
            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                cancelOrderInternal(orderId, true);
                return;
            }
            throw new RuntimeException("Cancel order failed: " + e.getMessage(), e);
        }
    }

    public List<Map<String, Object>> getMarketMetricsBatch(List<String> symbols) {
        return getMarketMetricsBatchInternal(symbols, false);
    }

    @SuppressWarnings("unchecked")
    private List<Map<String, Object>> getMarketMetricsBatchInternal(List<String> symbols, boolean isRetry) {
        if (accessToken == null)
            refreshAccessToken();

        List<Map<String, Object>> allItems = new ArrayList<>();
        int chunkSize = 250;

        for (int i = 0; i < symbols.size(); i += chunkSize) {
            List<String> chunk = symbols.subList(i, Math.min(i + chunkSize, symbols.size()));
            
            try {
                Map<String, Object> response = tastyTradeRestClient
                        .get()
                        .uri(uriBuilder -> uriBuilder
                                .path("/market-metrics")
                                .queryParam("symbols", chunk)
                                .build())
                        .header("Authorization", "Bearer " + accessToken)
                        .retrieve()
                        .body(Map.class);

                if (response != null && response.containsKey("data")) {
                    Map<String, Object> data = (Map<String, Object>) response.get("data");
                    List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
                    if (items != null) {
                        if (!items.isEmpty() && log.isInfoEnabled()) {
                            log.info("Market metrics keys for {}: {}", items.get(0).get("symbol"), items.get(0).keySet());
                        }
                        allItems.addAll(items);
                    }
                }
            } catch (Exception e) {
                log.error("Failed to get market metrics batch chunk starting at index {}: {}", i, e.getMessage());
                if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                    log.info("Token expired during market metrics batch, refreshing and retrying...");
                    refreshAccessToken();
                    return getMarketMetricsBatchInternal(symbols, true);
                }
            }
        }

        return allItems;
    }

    /**
     * Obtiene velas historicas utilizando el API REST (Punto 2 del Manual).
     * Endpoint: /market-data/candles
     */
    @SuppressWarnings("unchecked")
    public List<Candle> getHistoricalCandles(String symbol, String dxSymbol, Instant from, Instant to) {
        if (accessToken == null) refreshAccessToken();

        try {
            // Construir la URI manualmente para evitar que Spring intente expandir las llaves { }
            String url = org.springframework.web.util.UriComponentsBuilder.fromPath("/market-data/candles")
                    .queryParam("symbol", dxSymbol)
                    .queryParam("from-date", from.atOffset(ZoneOffset.UTC).toString())
                    .queryParam("to-date", to.atOffset(ZoneOffset.UTC).toString())
                    .build()
                    .encode()
                    .toUriString();

            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri(java.net.URI.create(url))
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(Map.class);

            if (response == null || !response.containsKey("data")) {
                return List.of();
            }

            Map<String, Object> data = (Map<String, Object>) response.get("data");
            List<Map<String, Object>> items = (List<Map<String, Object>>) data.get("items");
            if (items == null) return List.of();

            List<Candle> candles = new ArrayList<>();
            for (Map<String, Object> item : items) {
                Candle c = new Candle();
                c.setSymbol(symbol);
                // The API returns timestamp as long (milliseconds)
                Number time = (Number) item.get("time");
                if (time != null) {
                    // El API de TastyTrade devuelve el tiempo en microsegundos o milisegundos.
                    // Generalmente el dxFeed usa ms para bars y micros para ticks.
                    // Si el numero es muy grande, puede ser micros.
                    long t = time.longValue();
                    c.setTimestamp(Instant.ofEpochMilli(t > 100000000000000L ? t / 1000 : t));
                }
                c.setOpen(((Number) item.get("open")).doubleValue());
                c.setHigh(((Number) item.get("high")).doubleValue());
                c.setLow(((Number) item.get("low")).doubleValue());
                c.setClose(((Number) item.get("close")).doubleValue());
                c.setVolume(((Number) item.get("volume")).doubleValue());
                candles.add(c);
            }
            return candles;
        } catch (Exception e) {
            log.error("Failed to get historical candles for {}: {}", symbol, e.getMessage());
            return List.of();
        }
    }

    /**
     * Obtiene el calendario de sesiones de mercado (Punto 2 del Refinamiento).
     * Endpoint: /market-time/sessions
     */
    @SuppressWarnings("unchecked")
    public List<Map<String, Object>> getMarketTimeSessions(String fromDate, String toDate) {
        if (accessToken == null) refreshAccessToken();

        try {
            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri(uriBuilder -> uriBuilder
                            .path("/market-time/sessions")
                            .queryParam("from-date", fromDate)
                            .queryParam("to-date", toDate)
                            .build())
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
            log.error("Failed to get market time sessions: {}", e.getMessage());
            return List.of();
        }
    }

    /**
     * Obtiene el calendario de dias festivos (Punto 2 del Refinamiento).
     * Endpoint: /market-time/equities/holidays
     */
    @SuppressWarnings("unchecked")
    public List<Map<String, Object>> getMarketTimeHolidays() {
        if (accessToken == null) refreshAccessToken();

        try {
            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri("/market-time/equities/holidays")
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
            log.error("Failed to get market time holidays: {}", e.getMessage());
            return List.of();
        }
    }
    /**
     * Obtiene la cadena de opciones anidada (expiraciones y strikes).
     * GET /option-chains/{symbol}/nested
     */
    public Map<String, Object> getOptionChainNested(String symbol) {
        return getOptionChainNestedInternal(symbol, false);
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> getOptionChainNestedInternal(String symbol, boolean isRetry) {
        if (accessToken == null) refreshAccessToken();

        try {
            Map<String, Object> response = tastyTradeRestClient
                    .get()
                    .uri("/option-chains/{symbol}/nested", symbol)
                    .header("Authorization", "Bearer " + accessToken)
                    .retrieve()
                    .body(Map.class);

            if (response == null || !response.containsKey("data")) {
                return Map.of();
            }

            return (Map<String, Object>) response.get("data");
        } catch (Exception e) {
            log.error("Failed to get option chain for {}: {}", symbol, e.getMessage());
            if (!isRetry && e.getMessage() != null && e.getMessage().contains("401")) {
                refreshAccessToken();
                return getOptionChainNestedInternal(symbol, true);
            }
            return Map.of();
        }
    }

    private <T> T executeWithBackoff(java.util.function.Supplier<T> action, int retryCount) {
        if (retryCount > MAX_RETRIES) {
            log.error("Max retries reached for 429 error.");
            return action.get();
        }
        
        long waitTime = INITIAL_BACKOFF_MS * (long) Math.pow(2, retryCount - 1);
        log.info("Backoff retry {}/{} - waiting {}ms", retryCount, MAX_RETRIES, waitTime);
        
        try {
            Thread.sleep(waitTime);
        } catch (InterruptedException ie) {
            Thread.currentThread().interrupt();
        }
        
        try {
            return action.get();
        } catch (Exception e) {
            if (e.getMessage() != null && e.getMessage().contains("429")) {
                return executeWithBackoff(action, retryCount + 1);
            }
            throw e;
        }
    }
}
