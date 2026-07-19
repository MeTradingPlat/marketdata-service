package com.metradingplat.marketdata.application.service;

import com.metradingplat.marketdata.adapter.external.tastytrade.TastytradeFacadeAdapter;
import com.metradingplat.marketdata.application.dto.BalanceDTO;
import com.metradingplat.marketdata.application.dto.ComplexOrderDryRunDTO;
import com.metradingplat.marketdata.application.dto.MarketMetricsDTO;
import com.metradingplat.marketdata.application.dto.OptionChainDTO;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

/**
 * Unit tests for TastytradeFacadeService.
 * Verifies orchestration logic, delegation to adapter, and error handling.
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("TastytradeFacadeService Tests")
class TastytradeFacadeServiceTest {

    @Mock
    private TastytradeFacadeAdapter adapter;

    private TastytradeFacadeService service;

    @BeforeEach
    void setUp() {
        service = new TastytradeFacadeService(adapter);
    }

    // ─── getAccountBalances ───────────────────────────────────────────────────

    @Test
    @DisplayName("shouldReturnBalanceWhenAdapterReturnsValidData")
    void shouldReturnBalanceWhenAdapterReturnsValidData() {
        // Arrange
        BalanceDTO expected = BalanceDTO.builder()
            .accountNumber("5YZ12345")
            .cashBalance(new BigDecimal("100000.00"))
            .buyingPower(new BigDecimal("200000.00"))
            .build();
        when(adapter.getAccountBalances("5YZ12345")).thenReturn(expected);

        // Act
        BalanceDTO result = service.getAccountBalances("5YZ12345");

        // Assert
        assertThat(result).isNotNull();
        assertThat(result.getAccountNumber()).isEqualTo("5YZ12345");
        assertThat(result.getCashBalance()).isEqualByComparingTo("100000.00");
        verify(adapter, times(1)).getAccountBalances("5YZ12345");
    }

    @Test
    @DisplayName("shouldReturnNullWhenAdapterReturnsNullBalance")
    void shouldReturnNullWhenAdapterReturnsNullBalance() {
        // Arrange
        when(adapter.getAccountBalances(anyString())).thenReturn(null);

        // Act
        BalanceDTO result = service.getAccountBalances("5YZ12345");

        // Assert
        assertThat(result).isNull();
    }

    @Test
    @DisplayName("shouldWrapAdapterExceptionInRuntimeExceptionWhenBalanceFetchFails")
    void shouldWrapAdapterExceptionInRuntimeExceptionWhenBalanceFetchFails() {
        // Arrange
        when(adapter.getAccountBalances(anyString()))
            .thenThrow(new RuntimeException("Tastytrade API timeout"));

        // Act & Assert
        assertThatThrownBy(() -> service.getAccountBalances("5YZ12345"))
            .isInstanceOf(RuntimeException.class)
            .hasMessageContaining("Failed to fetch account balances");
    }

    // ─── getOptionChainNested ─────────────────────────────────────────────────

    @Test
    @DisplayName("shouldReturnOptionChainWhenAdapterReturnsValidData")
    void shouldReturnOptionChainWhenAdapterReturnsValidData() {
        // Arrange
        OptionChainDTO expected = OptionChainDTO.builder()
            .symbol("SPY")
            .expirations(List.of())
            .build();
        when(adapter.getOptionChainNested("SPY")).thenReturn(expected);

        // Act
        OptionChainDTO result = service.getOptionChainNested("SPY");

        // Assert
        assertThat(result).isNotNull();
        assertThat(result.getSymbol()).isEqualTo("SPY");
        verify(adapter, times(1)).getOptionChainNested("SPY");
    }

    @Test
    @DisplayName("shouldReturnNullWhenAdapterReturnsNullOptionChain")
    void shouldReturnNullWhenAdapterReturnsNullOptionChain() {
        // Arrange
        when(adapter.getOptionChainNested(anyString())).thenReturn(null);

        // Act
        OptionChainDTO result = service.getOptionChainNested("UNKNOWN");

        // Assert
        assertThat(result).isNull();
    }

    @Test
    @DisplayName("shouldWrapAdapterExceptionWhenOptionChainFetchFails")
    void shouldWrapAdapterExceptionWhenOptionChainFetchFails() {
        // Arrange
        when(adapter.getOptionChainNested(anyString()))
            .thenThrow(new RuntimeException("Symbol not found"));

        // Act & Assert
        assertThatThrownBy(() -> service.getOptionChainNested("INVALID"))
            .isInstanceOf(RuntimeException.class)
            .hasMessageContaining("Failed to fetch option chain");
    }

    // ─── getMarketMetrics ─────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldReturnMarketMetricsWhenAdapterReturnsValidData")
    void shouldReturnMarketMetricsWhenAdapterReturnsValidData() {
        // Arrange
        MarketMetricsDTO expected = MarketMetricsDTO.builder()
            .vix(new BigDecimal("18.50"))
            .putCallRatio(new BigDecimal("0.85"))
            .build();
        when(adapter.getMarketMetrics()).thenReturn(expected);

        // Act
        MarketMetricsDTO result = service.getMarketMetrics();

        // Assert
        assertThat(result).isNotNull();
        assertThat(result.getVix()).isEqualByComparingTo("18.50");
        verify(adapter, times(1)).getMarketMetrics();
    }

    @Test
    @DisplayName("shouldReturnNullWhenAdapterReturnsNullMarketMetrics")
    void shouldReturnNullWhenAdapterReturnsNullMarketMetrics() {
        // Arrange
        when(adapter.getMarketMetrics()).thenReturn(null);

        // Act
        MarketMetricsDTO result = service.getMarketMetrics();

        // Assert
        assertThat(result).isNull();
    }

    @Test
    @DisplayName("shouldWrapAdapterExceptionWhenMarketMetricsFetchFails")
    void shouldWrapAdapterExceptionWhenMarketMetricsFetchFails() {
        // Arrange
        when(adapter.getMarketMetrics())
            .thenThrow(new RuntimeException("Tastytrade API unavailable"));

        // Act & Assert
        assertThatThrownBy(() -> service.getMarketMetrics())
            .isInstanceOf(RuntimeException.class)
            .hasMessageContaining("Failed to fetch market metrics");
    }

    // ─── dryRunComplexOrder ───────────────────────────────────────────────────

    @Test
    @DisplayName("shouldReturnDryRunResultWhenAdapterValidatesOrderSuccessfully")
    void shouldReturnDryRunResultWhenAdapterValidatesOrderSuccessfully() {
        // Arrange
        ComplexOrderDryRunDTO request = ComplexOrderDryRunDTO.builder()
            .orderType("OTOCO")
            .build();
        ComplexOrderDryRunDTO expected = ComplexOrderDryRunDTO.builder()
            .isValid(true)
            .estimatedFees(new BigDecimal("2.50"))
            .marginRequirement(new BigDecimal("5000.00"))
            .build();
        when(adapter.dryRunComplexOrder(any())).thenReturn(expected);

        // Act
        ComplexOrderDryRunDTO result = service.dryRunComplexOrder(request);

        // Assert
        assertThat(result).isNotNull();
        assertThat(result.getIsValid()).isTrue();
        assertThat(result.getEstimatedFees()).isEqualByComparingTo("2.50");
        verify(adapter, times(1)).dryRunComplexOrder(request);
    }

    @Test
    @DisplayName("shouldReturnNullWhenAdapterReturnNullDryRunResult")
    void shouldReturnNullWhenAdapterReturnNullDryRunResult() {
        // Arrange
        when(adapter.dryRunComplexOrder(any())).thenReturn(null);

        // Act
        ComplexOrderDryRunDTO result = service.dryRunComplexOrder(new ComplexOrderDryRunDTO());

        // Assert
        assertThat(result).isNull();
    }

    @Test
    @DisplayName("shouldWrapAdapterExceptionWhenDryRunFails")
    void shouldWrapAdapterExceptionWhenDryRunFails() {
        // Arrange — HTTP 422 del broker
        when(adapter.dryRunComplexOrder(any()))
            .thenThrow(new RuntimeException("Order structure invalid: missing type field"));

        // Act & Assert
        assertThatThrownBy(() -> service.dryRunComplexOrder(new ComplexOrderDryRunDTO()))
            .isInstanceOf(RuntimeException.class)
            .hasMessageContaining("Failed to dry run complex order");
    }
}
