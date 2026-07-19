package com.metradingplat.marketdata.domain.control_plane.entity;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;

import static org.assertj.core.api.Assertions.*;

/**
 * Unit tests for Order entity validation logic.
 * Tests order creation, validation, and state transitions.
 * Target coverage: >= 80%
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("Order Validation Tests")
class OrderValidationTest {

    private Order order;

    @BeforeEach
    void setUp() {
        order = new Order("SPY", 100, new BigDecimal("450.00"), Order.OrderSide.BUY);
    }

    @Test
    @DisplayName("shouldCreateOrderWhenValidParametersProvided")
    void shouldCreateOrderWhenValidParametersProvided() {
        // Arrange & Act
        Order createdOrder = new Order("AAPL", 50, new BigDecimal("150.00"), Order.OrderSide.SELL);

        // Assert
        assertThat(createdOrder).isNotNull();
        assertThat(createdOrder.getSymbol()).isEqualTo("AAPL");
        assertThat(createdOrder.getQuantity()).isEqualTo(50);
        assertThat(createdOrder.getPrice()).isEqualByComparingTo(new BigDecimal("150.00"));
        assertThat(createdOrder.getSide()).isEqualTo(Order.OrderSide.SELL);
        assertThat(createdOrder.getStatus()).isEqualTo(Order.OrderStatus.PENDING);
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenSymbolIsNull")
    void shouldThrowExceptionWhenSymbolIsNull() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Order(null, 100, new BigDecimal("450.00"), Order.OrderSide.BUY))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("symbol");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenQuantityIsZero")
    void shouldThrowExceptionWhenQuantityIsZero() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Order("SPY", 0, new BigDecimal("450.00"), Order.OrderSide.BUY))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("quantity");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenQuantityIsNegative")
    void shouldThrowExceptionWhenQuantityIsNegative() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Order("SPY", -100, new BigDecimal("450.00"), Order.OrderSide.BUY))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("quantity");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenPriceIsNull")
    void shouldThrowExceptionWhenPriceIsNull() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Order("SPY", 100, null, Order.OrderSide.BUY))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("price");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenPriceIsNegative")
    void shouldThrowExceptionWhenPriceIsNegative() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Order("SPY", 100, new BigDecimal("-450.00"), Order.OrderSide.BUY))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("price");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenSideIsNull")
    void shouldThrowExceptionWhenSideIsNull() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Order("SPY", 100, new BigDecimal("450.00"), null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("side");
    }

    @Test
    @DisplayName("shouldTransitionFromPendingToFilled")
    void shouldTransitionFromPendingToFilled() {
        // Arrange
        assertThat(order.getStatus()).isEqualTo(Order.OrderStatus.PENDING);

        // Act
        order.setStatus(Order.OrderStatus.FILLED);

        // Assert
        assertThat(order.getStatus()).isEqualTo(Order.OrderStatus.FILLED);
    }

    @Test
    @DisplayName("shouldTransitionFromPendingToCancelled")
    void shouldTransitionFromPendingToCancelled() {
        // Arrange
        assertThat(order.getStatus()).isEqualTo(Order.OrderStatus.PENDING);

        // Act
        order.setStatus(Order.OrderStatus.CANCELLED);

        // Assert
        assertThat(order.getStatus()).isEqualTo(Order.OrderStatus.CANCELLED);
    }

    @Test
    @DisplayName("shouldCalculateTotalOrderValue")
    void shouldCalculateTotalOrderValue() {
        // Arrange
        Order testOrder = new Order("SPY", 100, new BigDecimal("450.00"), Order.OrderSide.BUY);

        // Act
        BigDecimal totalValue = testOrder.getQuantity() * testOrder.getPrice().doubleValue();

        // Assert
        assertThat(totalValue).isEqualTo(45000.0);
    }

    @Test
    @DisplayName("shouldHandleLargeQuantities")
    void shouldHandleLargeQuantities() {
        // Arrange & Act
        Order largeOrder = new Order("SPY", 1_000_000, new BigDecimal("450.00"), Order.OrderSide.BUY);

        // Assert
        assertThat(largeOrder.getQuantity()).isEqualTo(1_000_000);
    }

    @Test
    @DisplayName("shouldHandleFractionalPrices")
    void shouldHandleFractionalPrices() {
        // Arrange & Act
        Order fractionalOrder = new Order("SPY", 100, new BigDecimal("450.123456"), Order.OrderSide.BUY);

        // Assert
        assertThat(fractionalOrder.getPrice()).isEqualByComparingTo(new BigDecimal("450.123456"));
    }

    @Test
    @DisplayName("shouldSupportBothBuyAndSellSides")
    void shouldSupportBothBuyAndSellSides() {
        // Arrange & Act
        Order buyOrder = new Order("SPY", 100, new BigDecimal("450.00"), Order.OrderSide.BUY);
        Order sellOrder = new Order("SPY", 100, new BigDecimal("450.00"), Order.OrderSide.SELL);

        // Assert
        assertThat(buyOrder.getSide()).isEqualTo(Order.OrderSide.BUY);
        assertThat(sellOrder.getSide()).isEqualTo(Order.OrderSide.SELL);
    }
}
