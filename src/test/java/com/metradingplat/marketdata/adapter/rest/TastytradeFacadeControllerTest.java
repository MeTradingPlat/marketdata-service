package com.metradingplat.marketdata.adapter.rest;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.metradingplat.marketdata.application.dto.BalanceDTO;
import com.metradingplat.marketdata.application.dto.ComplexOrderDryRunDTO;
import com.metradingplat.marketdata.application.dto.MarketMetricsDTO;
import com.metradingplat.marketdata.application.dto.OptionChainDTO;
import com.metradingplat.marketdata.application.service.TastytradeFacadeService;
import com.metradingplat.marketdata.configuration.logging.TraceIdProvider;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.setup.MockMvcBuilders;

import java.math.BigDecimal;
import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

/**
 * Unit tests for TastytradeFacadeController.
 * Verifies HTTP status codes, response bodies, and delegation to service layer.
 * Uses standalone MockMvc (no Spring context needed).
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("TastytradeFacadeController Tests")
class TastytradeFacadeControllerTest {

    @Mock
    private TastytradeFacadeService service;

    @Mock
    private TraceIdProvider traceIdProvider;

    private MockMvc mockMvc;
    private final ObjectMapper objectMapper = new ObjectMapper();

    @BeforeEach
    void setUp() {
        TastytradeFacadeController controller =
            new TastytradeFacadeController(service, traceIdProvider);
        mockMvc = MockMvcBuilders.standaloneSetup(controller).build();
        when(traceIdProvider.getTraceId()).thenReturn("trace-test-001");
    }

    // ─── GET /accounts/{accountNumber}/balances ───────────────────────────────

    @Test
    @DisplayName("shouldReturn200WithBalanceWhenAccountExists")
    void shouldReturn200WithBalanceWhenAccountExists() throws Exception {
        // Arrange
        BalanceDTO balance = BalanceDTO.builder()
            .accountNumber("5YZ12345")
            .cashBalance(new BigDecimal("100000.00"))
            .buyingPower(new BigDecimal("200000.00"))
            .marginRequirement(new BigDecimal("5000.00"))
            .build();
        when(service.getAccountBalances("5YZ12345")).thenReturn(balance);

        // Act & Assert
        mockMvc.perform(get("/api/v1/accounts/5YZ12345/balances"))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$.accountNumber").value("5YZ12345"))
            .andExpect(jsonPath("$.cashBalance").value(100000.00))
            .andExpect(jsonPath("$.buyingPower").value(200000.00));

        verify(service).getAccountBalances("5YZ12345");
    }

    @Test
    @DisplayName("shouldReturn404WhenAccountBalancesNotFound")
    void shouldReturn404WhenAccountBalancesNotFound() throws Exception {
        // Arrange
        when(service.getAccountBalances("UNKNOWN")).thenReturn(null);

        // Act & Assert
        mockMvc.perform(get("/api/v1/accounts/UNKNOWN/balances"))
            .andExpect(status().isNotFound());
    }

    @Test
    @DisplayName("shouldReturn500WhenServiceThrowsExceptionOnBalances")
    void shouldReturn500WhenServiceThrowsExceptionOnBalances() throws Exception {
        // Arrange
        when(service.getAccountBalances(anyString()))
            .thenThrow(new RuntimeException("Tastytrade API down"));

        // Act & Assert
        mockMvc.perform(get("/api/v1/accounts/5YZ12345/balances"))
            .andExpect(status().is5xxServerError());
    }

    // ─── GET /option-chains/{symbol}/nested ──────────────────────────────────

    @Test
    @DisplayName("shouldReturn200WithOptionChainWhenSymbolIsValid")
    void shouldReturn200WithOptionChainWhenSymbolIsValid() throws Exception {
        // Arrange
        OptionChainDTO chain = OptionChainDTO.builder()
            .symbol("SPY")
            .expirations(List.of())
            .build();
        when(service.getOptionChainNested("SPY")).thenReturn(chain);

        // Act & Assert
        mockMvc.perform(get("/api/v1/option-chains/SPY/nested"))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$.symbol").value("SPY"));

        verify(service).getOptionChainNested("SPY");
    }

    @Test
    @DisplayName("shouldReturn404WhenOptionChainSymbolNotFound")
    void shouldReturn404WhenOptionChainSymbolNotFound() throws Exception {
        // Arrange
        when(service.getOptionChainNested("INVALID")).thenReturn(null);

        // Act & Assert
        mockMvc.perform(get("/api/v1/option-chains/INVALID/nested"))
            .andExpect(status().isNotFound());
    }

    // ─── GET /market-metrics ─────────────────────────────────────────────────

    @Test
    @DisplayName("shouldReturn200WithMarketMetricsWhenDataIsAvailable")
    void shouldReturn200WithMarketMetricsWhenDataIsAvailable() throws Exception {
        // Arrange
        MarketMetricsDTO metrics = MarketMetricsDTO.builder()
            .vix(new BigDecimal("18.50"))
            .putCallRatio(new BigDecimal("0.85"))
            .marketSentiment("NEUTRAL")
            .build();
        when(service.getMarketMetrics()).thenReturn(metrics);

        // Act & Assert
        mockMvc.perform(get("/api/v1/market-metrics"))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$.vix").value(18.50))
            .andExpect(jsonPath("$.marketSentiment").value("NEUTRAL"));

        verify(service).getMarketMetrics();
    }

    @Test
    @DisplayName("shouldReturn503WhenMarketMetricsAreUnavailable")
    void shouldReturn503WhenMarketMetricsAreUnavailable() throws Exception {
        // Arrange — sandbox deshabilita este endpoint
        when(service.getMarketMetrics()).thenReturn(null);

        // Act & Assert
        mockMvc.perform(get("/api/v1/market-metrics"))
            .andExpect(status().isServiceUnavailable());
    }

    // ─── POST /complex-orders/dry-run ─────────────────────────────────────────

    @Test
    @DisplayName("shouldReturn200WithValidationResultWhenOrderIsValid")
    void shouldReturn200WithValidationResultWhenOrderIsValid() throws Exception {
        // Arrange
        ComplexOrderDryRunDTO request = ComplexOrderDryRunDTO.builder()
            .orderType("OTOCO")
            .legs(List.of(
                ComplexOrderDryRunDTO.OrderLegDTO.builder()
                    .symbol("SPY").quantity(1L).side("BUY").build()
            ))
            .build();
        ComplexOrderDryRunDTO result = ComplexOrderDryRunDTO.builder()
            .isValid(true)
            .estimatedFees(new BigDecimal("2.50"))
            .marginRequirement(new BigDecimal("5000.00"))
            .build();
        when(service.dryRunComplexOrder(any())).thenReturn(result);

        // Act & Assert
        mockMvc.perform(post("/api/v1/complex-orders/dry-run")
                .contentType(MediaType.APPLICATION_JSON)
                .content(objectMapper.writeValueAsString(request)))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$.isValid").value(true))
            .andExpect(jsonPath("$.estimatedFees").value(2.50));
    }

    @Test
    @DisplayName("shouldReturn400WhenDryRunReturnsNullResult")
    void shouldReturn400WhenDryRunReturnsNullResult() throws Exception {
        // Arrange — orden con estructura inválida
        when(service.dryRunComplexOrder(any())).thenReturn(null);

        // Act & Assert
        mockMvc.perform(post("/api/v1/complex-orders/dry-run")
                .contentType(MediaType.APPLICATION_JSON)
                .content("{}"))
            .andExpect(status().isBadRequest());
    }

    @Test
    @DisplayName("shouldReturn500WhenDryRunServiceThrowsException")
    void shouldReturn500WhenDryRunServiceThrowsException() throws Exception {
        // Arrange — HTTP 422 del broker envuelto en RuntimeException
        when(service.dryRunComplexOrder(any()))
            .thenThrow(new RuntimeException("Order structure invalid: missing type field"));

        // Act & Assert
        mockMvc.perform(post("/api/v1/complex-orders/dry-run")
                .contentType(MediaType.APPLICATION_JSON)
                .content("{}"))
            .andExpect(status().is5xxServerError());
    }
}
