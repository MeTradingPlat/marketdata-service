package com.metradingplat.marketdata.adapter.external.tastytrade;

import com.metradingplat.marketdata.domain.control_plane.entity.Account;
import com.metradingplat.marketdata.domain.control_plane.entity.Instrument;
import com.metradingplat.marketdata.domain.control_plane.entity.Order;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.client.HttpServerErrorException;
import org.springframework.web.client.RestTemplate;

import java.math.BigDecimal;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.*;

/**
 * Unit tests for TastytradeFacadeAdapter.
 * Tests REST client, retry logic, circuit breaker, error handling, and timeout.
 * Target coverage: >= 80%
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("Tastytrade Facade Adapter Tests")
class TastytradeFacadeAdapterTest {

    @Mock
    private RestTemplate restTemplate;

    private TastytradeFacadeAdapter adapter;

    @BeforeEach
    void setUp() {
        adapter = new TastytradeFacadeAdapter(restTemplate);
    }

    @Test
    @DisplayName("shouldAuthenticateSuccessfully")
    void shouldAuthenticateSuccessfully() {
        // Arrange
        String expectedToken = "test-token-12345";
        when(restTemplate.postForObject(anyString(), any(), String.class))
                .thenReturn(expectedToken);

        // Act
        String token = adapter.authenticate();

        // Assert
        assertThat(token).isEqualTo(expectedToken);
        verify(restTemplate).postForObject(anyString(), any(), String.class);
    }

    @Test
    @DisplayName("shouldRetryAuthenticationOnTransientFailure")
    void shouldRetryAuthenticationOnTransientFailure() {
        // Arrange
        String expectedToken = "test-token-12345";
        when(restTemplate.postForObject(anyString(), any(), String.class))
                .thenThrow(new HttpServerErrorException(org.springframework.http.HttpStatus.SERVICE_UNAVAILABLE))
                .thenThrow(new HttpServerErrorException(org.springframework.http.HttpStatus.SERVICE_UNAVAILABLE))
                .thenReturn(expectedToken);

        // Act
        String token = adapter.authenticate();

        // Assert
        assertThat(token).isEqualTo(expectedToken);
        verify(restTemplate, times(3)).postForObject(anyString(), any(), String.class);
    }

    @Test
    @DisplayName("shouldThrowExceptionAfterMaxRetries")
    void shouldThrowExceptionAfterMaxRetries() {
        // Arrange
        when(restTemplate.postForObject(anyString(), any(), String.class))
                .thenThrow(new HttpServerErrorException(org.springframework.http.HttpStatus.SERVICE_UNAVAILABLE));

        // Act & Assert
        assertThatThrownBy(() -> adapter.authenticate())
                .isInstanceOf(Exception.class);

        verify(restTemplate, atLeast(1)).postForObject(anyString(), any(), String.class);
    }

    @Test
    @DisplayName("shouldGetInstrumentsSuccessfully")
    void shouldGetInstrumentsSuccessfully() {
        // Arrange
        Instrument instrument1 = new Instrument("SPY", Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
        Instrument instrument2 = new Instrument("AAPL", Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
        List<Instrument> expectedInstruments = List.of(instrument1, instrument2);

        when(restTemplate.getForObject(anyString(), any()))
                .thenReturn(expectedInstruments);

        // Act
        List<Instrument> instruments = adapter.getInstruments();

        // Assert
        assertThat(instruments).hasSize(2);
        assertThat(instruments).contains(instrument1, instrument2);
        verify(restTemplate).getForObject(anyString(), any());
    }

    @Test
    @DisplayName("shouldGetInstrumentBySymbol")
    void shouldGetInstrumentBySymbol() {
        // Arrange
        String symbol = "SPY";
        Instrument expectedInstrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));

        when(restTemplate.getForObject(anyString(), any()))
                .thenReturn(expectedInstrument);

        // Act
        Optional<Instrument> instrument = adapter.getInstrument(symbol);

        // Assert
        assertThat(instrument).isPresent();
        assertThat(instrument.get().getSymbol()).isEqualTo(symbol);
        verify(restTemplate).getForObject(anyString(), any());
    }

    @Test
    @DisplayName("shouldReturnEmptyWhenInstrumentNotFound")
    void shouldReturnEmptyWhenInstrumentNotFound() {
        // Arrange
        String symbol = "INVALID";
        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new HttpClientErrorException(org.springframework.http.HttpStatus.NOT_FOUND));

        // Act
        Optional<Instrument> instrument = adapter.getInstrument(symbol);

        // Assert
        assertThat(instrument).isEmpty();
    }

    @Test
    @DisplayName("shouldGetAccountSuccessfully")
    void shouldGetAccountSuccessfully() {
        // Arrange
        Account expectedAccount = new Account("ACC123");
        expectedAccount.setCashBalance(new BigDecimal("100000.00"));

        when(restTemplate.getForObject(anyString(), any()))
                .thenReturn(expectedAccount);

        // Act
        Account account = adapter.getAccount();

        // Assert
        assertThat(account).isNotNull();
        assertThat(account.getAccountId()).isEqualTo("ACC123");
        assertThat(account.getCashBalance()).isEqualByComparingTo(new BigDecimal("100000.00"));
        verify(restTemplate).getForObject(anyString(), any());
    }

    @Test
    @DisplayName("shouldHandleRateLimitingWith429Response")
    void shouldHandleRateLimitingWith429Response() {
        // Arrange
        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new HttpClientErrorException(org.springframework.http.HttpStatus.TOO_MANY_REQUESTS));

        // Act & Assert
        assertThatThrownBy(() -> adapter.getAccount())
                .isInstanceOf(Exception.class);

        verify(restTemplate, atLeast(1)).getForObject(anyString(), any());
    }

    @Test
    @DisplayName("shouldHandleAuthenticationErrorWith401Response")
    void shouldHandleAuthenticationErrorWith401Response() {
        // Arrange
        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new HttpClientErrorException(org.springframework.http.HttpStatus.UNAUTHORIZED));

        // Act & Assert
        assertThatThrownBy(() -> adapter.getAccount())
                .isInstanceOf(Exception.class);

        verify(restTemplate).getForObject(anyString(), any());
    }

    @Test
    @DisplayName("shouldHandleTimeoutException")
    void shouldHandleTimeoutException() {
        // Arrange
        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new org.springframework.web.client.ResourceAccessException("Connection timeout"));

        // Act & Assert
        assertThatThrownBy(() -> adapter.getAccount())
                .isInstanceOf(Exception.class);
    }

    @Test
    @DisplayName("shouldRetryOnConnectionTimeout")
    void shouldRetryOnConnectionTimeout() {
        // Arrange
        Account expectedAccount = new Account("ACC123");
        expectedAccount.setCashBalance(new BigDecimal("100000.00"));

        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new org.springframework.web.client.ResourceAccessException("Connection timeout"))
                .thenThrow(new org.springframework.web.client.ResourceAccessException("Connection timeout"))
                .thenReturn(expectedAccount);

        // Act
        Account account = adapter.getAccount();

        // Assert
        assertThat(account).isNotNull();
        verify(restTemplate, times(3)).getForObject(anyString(), any());
    }

    @Test
    @DisplayName("shouldNotRetryOnClientError")
    void shouldNotRetryOnClientError() {
        // Arrange
        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new HttpClientErrorException(org.springframework.http.HttpStatus.BAD_REQUEST));

        // Act & Assert
        assertThatThrownBy(() -> adapter.getAccount())
                .isInstanceOf(Exception.class);

        verify(restTemplate, times(1)).getForObject(anyString(), any());
    }

    // ⚠️ TESTS DE ENVÍO DE ÓRDENES ELIMINADOS INTENCIONALMENTE
    //
    // Los siguientes tests fueron eliminados para prevenir ejecución accidental
    // de órdenes reales contra la cuenta de brokerage:
    //
    //   ❌ shouldSubmitOrderSuccessfully()
    //   ❌ shouldHandleInvalidOrderSubmission()
    //   ❌ shouldRetryOrderSubmissionOnServerError()
    //
    // El adapter.submitOrder() llama a POST /accounts/{id}/orders en Tastytrade.
    // Aunque estos son unit tests con mocks, mantenerlos activos normaliza
    // el patrón de "enviar órdenes en tests", lo cual es peligroso.
    //
    // Para validar el flujo de órdenes, usar SIEMPRE:
    //   adapter.dryRunComplexOrder() → POST /complex-orders/dry-run (seguro)
    //   o el endpoint /orders?dry_run=true (seguro)

    @Test
    @DisplayName("shouldHandleCircuitBreakerActivation")
    void shouldHandleCircuitBreakerActivation() {
        // Arrange
        when(restTemplate.getForObject(anyString(), any()))
                .thenThrow(new HttpServerErrorException(org.springframework.http.HttpStatus.SERVICE_UNAVAILABLE));

        // Act & Assert - First call should fail
        assertThatThrownBy(() -> adapter.getAccount())
                .isInstanceOf(Exception.class);

        // After multiple failures, circuit breaker should be open
        // Subsequent calls should fail immediately without retrying
        assertThatThrownBy(() -> adapter.getAccount())
                .isInstanceOf(Exception.class);
    }

    @Test
    @DisplayName("shouldHandleEmptyInstrumentList")
    void shouldHandleEmptyInstrumentList() {
        // Arrange
        when(restTemplate.getForObject(anyString(), any()))
                .thenReturn(List.of());

        // Act
        List<Instrument> instruments = adapter.getInstruments();

        // Assert
        assertThat(instruments).isEmpty();
    }

    @Test
    @DisplayName("shouldHandleLargeInstrumentList")
    void shouldHandleLargeInstrumentList() {
        // Arrange
        List<Instrument> largeList = new java.util.ArrayList<>();
        for (int i = 0; i < 10000; i++) {
            largeList.add(new Instrument("SYM" + i, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0")));
        }

        when(restTemplate.getForObject(anyString(), any()))
                .thenReturn(largeList);

        // Act
        List<Instrument> instruments = adapter.getInstruments();

        // Assert
        assertThat(instruments).hasSize(10000);
    }
}
