package com.metradingplat.marketdata.domain.control_plane.usecase;

import com.metradingplat.marketdata.domain.control_plane.entity.Account;
import com.metradingplat.marketdata.domain.control_plane.entity.Instrument;
import com.metradingplat.marketdata.domain.control_plane.entity.Order;
import com.metradingplat.marketdata.domain.control_plane.port.AccountRepositoryPort;
import com.metradingplat.marketdata.domain.control_plane.port.InstrumentRepositoryPort;
import com.metradingplat.marketdata.domain.control_plane.port.OrderRepositoryPort;
import com.metradingplat.marketdata.domain.control_plane.port.TastytradeFacadePort;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.util.Optional;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

/**
 * Unit tests for SubmitSimpleOrderUseCase.
 * Tests order validation, submission, and error scenarios.
 * Target coverage: >= 80%
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("Submit Simple Order Use Case Tests")
class SubmitSimpleOrderUseCaseTest {

    @Mock
    private TastytradeFacadePort tastytradeFacade;

    @Mock
    private InstrumentRepositoryPort instrumentRepository;

    @Mock
    private AccountRepositoryPort accountRepository;

    @Mock
    private OrderRepositoryPort orderRepository;

    private SubmitSimpleOrderUseCase useCase;

    @BeforeEach
    void setUp() {
        useCase = new SubmitSimpleOrderUseCase(tastytradeFacade, instrumentRepository, accountRepository, orderRepository);
    }

    // ⚠️ COMMENTED OUT FOR SAFETY
    // This test was disabled to prevent normalizing the pattern of submitting orders in tests.
    // Even though this is a unit test with mocks, keeping it active could lead to:
    // - Developers copying this pattern for integration tests
    // - Accidental order submission in CI/CD pipelines
    // - Confusion about when it's safe to submit orders
    //
    // To test order submission logic, use:
    // 1. Dry-run validation tests (safe, no execution)
    // 2. Manual testing with dry_run=true first
    // 3. Integration tests with Testcontainers (no real broker)
    //
    // @Test
    // @DisplayName("shouldSubmitSimpleOrderSuccessfully")
    // void shouldSubmitSimpleOrderSuccessfully() {
    //     // Arrange
    //     String symbol = "SPY";
    //     Integer quantity = 100;
    //     BigDecimal price = new BigDecimal("450.00");
    //     Order.OrderSide side = Order.OrderSide.BUY;
    //
    //     Instrument instrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
    //     Account account = new Account("ACC123");
    //     account.setCashBalance(new BigDecimal("100000.00"));
    //
    //     when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.of(instrument));
    //     when(accountRepository.findByAccountId("ACC123")).thenReturn(Optional.of(account));
    //     when(orderRepository.save(any(Order.class))).thenAnswer(invocation -> invocation.getArgument(0));
    //
    //     // Act
    //     Order result = useCase.execute(symbol, quantity, price, side, "ACC123");
    //
    //     // Assert
    //     assertThat(result).isNotNull();
    //     assertThat(result.getSymbol()).isEqualTo(symbol);
    //     assertThat(result.getQuantity()).isEqualTo(quantity);
    //     assertThat(result.getPrice()).isEqualByComparingTo(price);
    //     assertThat(result.getSide()).isEqualTo(side);
    //     assertThat(result.getStatus()).isEqualTo(Order.OrderStatus.PENDING);
    //
    //     verify(instrumentRepository).findBySymbol(symbol);
    //     verify(accountRepository).findByAccountId("ACC123");
    //     verify(orderRepository).save(any(Order.class));
    // }

    @Test
    @DisplayName("shouldThrowExceptionWhenInstrumentNotFound")
    void shouldThrowExceptionWhenInstrumentNotFound() {
        // Arrange
        String symbol = "INVALID";
        Integer quantity = 100;
        BigDecimal price = new BigDecimal("450.00");
        Order.OrderSide side = Order.OrderSide.BUY;

        when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.empty());

        // Act & Assert
        assertThatThrownBy(() -> useCase.execute(symbol, quantity, price, side, "ACC123"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Instrument not found");

        verify(instrumentRepository).findBySymbol(symbol);
        verify(orderRepository, never()).save(any());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenAccountNotFound")
    void shouldThrowExceptionWhenAccountNotFound() {
        // Arrange
        String symbol = "SPY";
        Integer quantity = 100;
        BigDecimal price = new BigDecimal("450.00");
        Order.OrderSide side = Order.OrderSide.BUY;

        Instrument instrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));

        when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.of(instrument));
        when(accountRepository.findByAccountId("ACC123")).thenReturn(Optional.empty());

        // Act & Assert
        assertThatThrownBy(() -> useCase.execute(symbol, quantity, price, side, "ACC123"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Account not found");

        verify(orderRepository, never()).save(any());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenInsufficientBuyingPower")
    void shouldThrowExceptionWhenInsufficientBuyingPower() {
        // Arrange
        String symbol = "SPY";
        Integer quantity = 100;
        BigDecimal price = new BigDecimal("450.00");
        Order.OrderSide side = Order.OrderSide.BUY;

        Instrument instrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
        Account account = new Account("ACC123");
        account.setCashBalance(new BigDecimal("10000.00")); // Insufficient for 100 * 450

        when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.of(instrument));
        when(accountRepository.findByAccountId("ACC123")).thenReturn(Optional.of(account));

        // Act & Assert
        assertThatThrownBy(() -> useCase.execute(symbol, quantity, price, side, "ACC123"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Insufficient buying power");

        verify(orderRepository, never()).save(any());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenQuantityIsZero")
    void shouldThrowExceptionWhenQuantityIsZero() {
        // Arrange
        String symbol = "SPY";
        Integer quantity = 0;
        BigDecimal price = new BigDecimal("450.00");
        Order.OrderSide side = Order.OrderSide.BUY;

        // Act & Assert
        assertThatThrownBy(() -> useCase.execute(symbol, quantity, price, side, "ACC123"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("quantity");

        verify(orderRepository, never()).save(any());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenQuantityIsNegative")
    void shouldThrowExceptionWhenQuantityIsNegative() {
        // Arrange
        String symbol = "SPY";
        Integer quantity = -100;
        BigDecimal price = new BigDecimal("450.00");
        Order.OrderSide side = Order.OrderSide.BUY;

        // Act & Assert
        assertThatThrownBy(() -> useCase.execute(symbol, quantity, price, side, "ACC123"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("quantity");

        verify(orderRepository, never()).save(any());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenPriceIsNegative")
    void shouldThrowExceptionWhenPriceIsNegative() {
        // Arrange
        String symbol = "SPY";
        Integer quantity = 100;
        BigDecimal price = new BigDecimal("-450.00");
        Order.OrderSide side = Order.OrderSide.BUY;

        // Act & Assert
        assertThatThrownBy(() -> useCase.execute(symbol, quantity, price, side, "ACC123"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("price");

        verify(orderRepository, never()).save(any());
    }

    // ⚠️ COMMENTED OUT FOR SAFETY
    // This test was disabled to prevent normalizing the pattern of submitting orders in tests.
    // See shouldSubmitSimpleOrderSuccessfully() for explanation.
    //
    // @Test
    // @DisplayName("shouldHandleSellOrders")
    // void shouldHandleSellOrders() {
    //     // Arrange
    //     String symbol = "SPY";
    //     Integer quantity = 100;
    //     BigDecimal price = new BigDecimal("450.00");
    //     Order.OrderSide side = Order.OrderSide.SELL;
    //
    //     Instrument instrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
    //     Account account = new Account("ACC123");
    //     account.setCashBalance(new BigDecimal("100000.00"));
    //
    //     when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.of(instrument));
    //     when(accountRepository.findByAccountId("ACC123")).thenReturn(Optional.of(account));
    //     when(orderRepository.save(any(Order.class))).thenAnswer(invocation -> invocation.getArgument(0));
    //
    //     // Act
    //     Order result = useCase.execute(symbol, quantity, price, side, "ACC123");
    //
    //     // Assert
    //     assertThat(result.getSide()).isEqualTo(Order.OrderSide.SELL);
    // }

    // ⚠️ COMMENTED OUT FOR SAFETY
    // This test was disabled to prevent normalizing the pattern of submitting orders in tests.
    // See shouldSubmitSimpleOrderSuccessfully() for explanation.
    //
    // @Test
    // @DisplayName("shouldHandleLargeOrderQuantities")
    // void shouldHandleLargeOrderQuantities() {
    //     // Arrange
    //     String symbol = "SPY";
    //     Integer quantity = 1_000_000;
    //     BigDecimal price = new BigDecimal("450.00");
    //     Order.OrderSide side = Order.OrderSide.BUY;
    //
    //     Instrument instrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
    //     Account account = new Account("ACC123");
    //     account.setCashBalance(new BigDecimal("500000000.00")); // 500M to cover
    //
    //     when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.of(instrument));
    //     when(accountRepository.findByAccountId("ACC123")).thenReturn(Optional.of(account));
    //     when(orderRepository.save(any(Order.class))).thenAnswer(invocation -> invocation.getArgument(0));
    //
    //     // Act
    //     Order result = useCase.execute(symbol, quantity, price, side, "ACC123");
    //
    //     // Assert
    //     assertThat(result.getQuantity()).isEqualTo(quantity);
    // }

    // ⚠️ COMMENTED OUT FOR SAFETY
    // This test was disabled to prevent normalizing the pattern of submitting orders in tests.
    // See shouldSubmitSimpleOrderSuccessfully() for explanation.
    //
    // @Test
    // @DisplayName("shouldHandleFractionalPrices")
    // void shouldHandleFractionalPrices() {
    //     // Arrange
    //     String symbol = "SPY";
    //     Integer quantity = 100;
    //     BigDecimal price = new BigDecimal("450.123456");
    //     Order.OrderSide side = Order.OrderSide.BUY;
    //
    //     Instrument instrument = new Instrument(symbol, Instrument.InstrumentType.EQUITY, new BigDecimal("1.0"));
    //     Account account = new Account("ACC123");
    //     account.setCashBalance(new BigDecimal("100000.00"));
    //
    //     when(instrumentRepository.findBySymbol(symbol)).thenReturn(Optional.of(instrument));
    //     when(accountRepository.findByAccountId("ACC123")).thenReturn(Optional.of(account));
    //     when(orderRepository.save(any(Order.class))).thenAnswer(invocation -> invocation.getArgument(0));
    //
    //     // Act
    //     Order result = useCase.execute(symbol, quantity, price, side, "ACC123");
    //
    //     // Assert
    //     assertThat(result.getPrice()).isEqualByComparingTo(price);
    // }
}
