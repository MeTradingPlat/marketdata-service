package com.metradingplat.marketdata.domain.control_plane.usecase;

import com.metradingplat.marketdata.domain.control_plane.entity.Order;
import net.jqwik.api.*;
import net.jqwik.api.constraints.IntRange;
import net.jqwik.api.constraints.Positive;

import java.math.BigDecimal;

import static org.assertj.core.api.Assertions.*;

/**
 * Property-based tests for order validation.
 * **Validates: Requirements 5.1, 5.2, 5.3**
 *
 * Properties tested:
 * - Valid orders pass validation
 * - Invalid orders fail validation with descriptive errors
 * - Order validation is deterministic
 */
@DisplayName("Order Validation Property Tests")
class OrderValidationPropertyTest {

    @Property
    @DisplayName("shouldAcceptValidOrdersWithPositiveQuantityAndPrice")
    void shouldAcceptValidOrdersWithPositiveQuantityAndPrice(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act & Assert
        Order order = new Order(symbol, quantity, price, side);
        assertThat(order).isNotNull();
        assertThat(order.getSymbol()).isEqualTo(symbol);
        assertThat(order.getQuantity()).isEqualTo(quantity);
        assertThat(order.getPrice()).isEqualByComparingTo(price);
        assertThat(order.getSide()).isEqualTo(side);
        assertThat(order.getStatus()).isEqualTo(Order.OrderStatus.PENDING);
    }

    @Property
    @DisplayName("shouldRejectOrdersWithZeroQuantity")
    void shouldRejectOrdersWithZeroQuantity(
            @ForAll String symbol,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act & Assert
        assertThatThrownBy(() -> new Order(symbol, 0, price, side))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("quantity");
    }

    @Property
    @DisplayName("shouldRejectOrdersWithNegativeQuantity")
    void shouldRejectOrdersWithNegativeQuantity(
            @ForAll String symbol,
            @ForAll @IntRange(min = -1000000, max = -1) Integer negativeQuantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act & Assert
        assertThatThrownBy(() -> new Order(symbol, negativeQuantity, price, side))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("quantity");
    }

    @Property
    @DisplayName("shouldRejectOrdersWithNegativePrice")
    void shouldRejectOrdersWithNegativePrice(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);
        BigDecimal negativePrice = new BigDecimal("-100.00");

        // Act & Assert
        assertThatThrownBy(() -> new Order(symbol, quantity, negativePrice, side))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("price");
    }

    @Property
    @DisplayName("shouldRejectOrdersWithNullSymbol")
    void shouldRejectOrdersWithNullSymbol(
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Act & Assert
        assertThatThrownBy(() -> new Order(null, quantity, price, side))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("symbol");
    }

    @Property
    @DisplayName("shouldRejectOrdersWithNullPrice")
    void shouldRejectOrdersWithNullPrice(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act & Assert
        assertThatThrownBy(() -> new Order(symbol, quantity, null, side))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("price");
    }

    @Property
    @DisplayName("shouldRejectOrdersWithNullSide")
    void shouldRejectOrdersWithNullSide(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act & Assert
        assertThatThrownBy(() -> new Order(symbol, quantity, price, null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("side");
    }

    @Property
    @DisplayName("shouldBeDeterministicForSameInputs")
    void shouldBeDeterministicForSameInputs(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act
        Order order1 = new Order(symbol, quantity, price, side);
        Order order2 = new Order(symbol, quantity, price, side);

        // Assert - Both orders should have identical properties
        assertThat(order1.getSymbol()).isEqualTo(order2.getSymbol());
        assertThat(order1.getQuantity()).isEqualTo(order2.getQuantity());
        assertThat(order1.getPrice()).isEqualByComparingTo(order2.getPrice());
        assertThat(order1.getSide()).isEqualTo(order2.getSide());
        assertThat(order1.getStatus()).isEqualTo(order2.getStatus());
    }

    @Property
    @DisplayName("shouldSupportBothBuyAndSellSidesConsistently")
    void shouldSupportBothBuyAndSellSidesConsistently(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act
        Order buyOrder = new Order(symbol, quantity, price, Order.OrderSide.BUY);
        Order sellOrder = new Order(symbol, quantity, price, Order.OrderSide.SELL);

        // Assert
        assertThat(buyOrder.getSide()).isEqualTo(Order.OrderSide.BUY);
        assertThat(sellOrder.getSide()).isEqualTo(Order.OrderSide.SELL);
        assertThat(buyOrder.getSide()).isNotEqualTo(sellOrder.getSide());
    }

    @Property
    @DisplayName("shouldHandleVeryLargeQuantities")
    void shouldHandleVeryLargeQuantities(
            @ForAll String symbol,
            @ForAll @IntRange(min = 1, max = Integer.MAX_VALUE) Integer quantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act & Assert
        Order order = new Order(symbol, quantity, price, side);
        assertThat(order.getQuantity()).isEqualTo(quantity);
    }

    @Property
    @DisplayName("shouldHandleFractionalPricesConsistently")
    void shouldHandleFractionalPricesConsistently(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act
        Order order = new Order(symbol, quantity, price, side);

        // Assert
        assertThat(order.getPrice()).isEqualByComparingTo(price);
    }

    @Property
    @DisplayName("shouldMaintainStatusAsPendingAfterCreation")
    void shouldMaintainStatusAsPendingAfterCreation(
            @ForAll String symbol,
            @ForAll @Positive Integer quantity,
            @ForAll @Positive BigDecimal price,
            @ForAll Order.OrderSide side) {

        // Arrange
        Assume.that(symbol != null && !symbol.isEmpty() && symbol.length() <= 10);

        // Act
        Order order = new Order(symbol, quantity, price, side);

        // Assert
        assertThat(order.getStatus()).isEqualTo(Order.OrderStatus.PENDING);
    }
}
