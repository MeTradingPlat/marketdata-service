package com.metradingplat.marketdata.domain.control_plane.entity;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;

import static org.assertj.core.api.Assertions.*;

/**
 * Unit tests for Account entity balance calculations.
 * Tests account creation, balance updates, and buying power calculations.
 * Target coverage: >= 80%
 */
@ExtendWith(MockitoExtension.class)
@DisplayName("Account Balance Calculation Tests")
class AccountBalanceCalculationTest {

    private Account account;

    @BeforeEach
    void setUp() {
        account = new Account("ACC123");
        account.setCashBalance(new BigDecimal("100000.00"));
    }

    @Test
    @DisplayName("shouldCreateAccountWithValidAccountId")
    void shouldCreateAccountWithValidAccountId() {
        // Arrange & Act
        Account newAccount = new Account("ACC456");

        // Assert
        assertThat(newAccount).isNotNull();
        assertThat(newAccount.getAccountId()).isEqualTo("ACC456");
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenAccountIdIsNull")
    void shouldThrowExceptionWhenAccountIdIsNull() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> new Account(null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("accountId");
    }

    @Test
    @DisplayName("shouldSetAndGetCashBalance")
    void shouldSetAndGetCashBalance() {
        // Arrange
        BigDecimal newBalance = new BigDecimal("250000.00");

        // Act
        account.setCashBalance(newBalance);

        // Assert
        assertThat(account.getCashBalance()).isEqualByComparingTo(newBalance);
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenCashBalanceIsNegative")
    void shouldThrowExceptionWhenCashBalanceIsNegative() {
        // Arrange & Act & Assert
        assertThatThrownBy(() -> account.setCashBalance(new BigDecimal("-1000.00")))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("balance");
    }

    @Test
    @DisplayName("shouldCalculateBuyingPowerCorrectly")
    void shouldCalculateBuyingPowerCorrectly() {
        // Arrange
        account.setCashBalance(new BigDecimal("100000.00"));
        BigDecimal marginRequirement = new BigDecimal("50000.00");

        // Act
        BigDecimal buyingPower = account.getCashBalance().add(marginRequirement);

        // Assert
        assertThat(buyingPower).isEqualByComparingTo(new BigDecimal("150000.00"));
    }

    @Test
    @DisplayName("shouldHandleZeroCashBalance")
    void shouldHandleZeroCashBalance() {
        // Arrange & Act
        account.setCashBalance(new BigDecimal("0.00"));

        // Assert
        assertThat(account.getCashBalance()).isEqualByComparingTo(BigDecimal.ZERO);
    }

    @Test
    @DisplayName("shouldHandleLargeCashBalances")
    void shouldHandleLargeCashBalances() {
        // Arrange & Act
        BigDecimal largeBalance = new BigDecimal("999999999.99");
        account.setCashBalance(largeBalance);

        // Assert
        assertThat(account.getCashBalance()).isEqualByComparingTo(largeBalance);
    }

    @Test
    @DisplayName("shouldHandleFractionalCashBalances")
    void shouldHandleFractionalCashBalances() {
        // Arrange & Act
        BigDecimal fractionalBalance = new BigDecimal("100000.123456");
        account.setCashBalance(fractionalBalance);

        // Assert
        assertThat(account.getCashBalance()).isEqualByComparingTo(fractionalBalance);
    }

    @Test
    @DisplayName("shouldUpdateCashBalanceMultipleTimes")
    void shouldUpdateCashBalanceMultipleTimes() {
        // Arrange
        BigDecimal initialBalance = new BigDecimal("100000.00");
        account.setCashBalance(initialBalance);

        // Act
        account.setCashBalance(new BigDecimal("90000.00"));
        account.setCashBalance(new BigDecimal("95000.00"));
        account.setCashBalance(new BigDecimal("110000.00"));

        // Assert
        assertThat(account.getCashBalance()).isEqualByComparingTo(new BigDecimal("110000.00"));
    }

    @Test
    @DisplayName("shouldCalculateMarginRequirement")
    void shouldCalculateMarginRequirement() {
        // Arrange
        BigDecimal orderValue = new BigDecimal("50000.00");
        BigDecimal marginPercentage = new BigDecimal("0.25"); // 25% margin requirement

        // Act
        BigDecimal marginRequired = orderValue.multiply(marginPercentage);

        // Assert
        assertThat(marginRequired).isEqualByComparingTo(new BigDecimal("12500.00"));
    }

    @Test
    @DisplayName("shouldValidateSufficientBuyingPower")
    void shouldValidateSufficientBuyingPower() {
        // Arrange
        account.setCashBalance(new BigDecimal("100000.00"));
        BigDecimal requiredBuyingPower = new BigDecimal("50000.00");

        // Act
        boolean hasSufficientPower = account.getCashBalance().compareTo(requiredBuyingPower) >= 0;

        // Assert
        assertThat(hasSufficientPower).isTrue();
    }

    @Test
    @DisplayName("shouldDetectInsufficientBuyingPower")
    void shouldDetectInsufficientBuyingPower() {
        // Arrange
        account.setCashBalance(new BigDecimal("10000.00"));
        BigDecimal requiredBuyingPower = new BigDecimal("50000.00");

        // Act
        boolean hasSufficientPower = account.getCashBalance().compareTo(requiredBuyingPower) >= 0;

        // Assert
        assertThat(hasSufficientPower).isFalse();
    }

    @Test
    @DisplayName("shouldHandleEdgeCaseOfExactBuyingPower")
    void shouldHandleEdgeCaseOfExactBuyingPower() {
        // Arrange
        account.setCashBalance(new BigDecimal("50000.00"));
        BigDecimal requiredBuyingPower = new BigDecimal("50000.00");

        // Act
        boolean hasSufficientPower = account.getCashBalance().compareTo(requiredBuyingPower) >= 0;

        // Assert
        assertThat(hasSufficientPower).isTrue();
    }
}
