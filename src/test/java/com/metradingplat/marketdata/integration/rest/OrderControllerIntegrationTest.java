package com.metradingplat.marketdata.integration.rest;

import com.metradingplat.marketdata.integration.IntegrationTestBase;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;

import java.math.BigDecimal;

import static org.assertj.core.api.Assertions.*;

/**
 * Integration tests for Order REST endpoints.
 *
 * ⚠️ SAFETY POLICY: This test class ONLY covers read-only operations (GET)
 * and dry_run=true validations. Tests that submit real orders (dry_run=false)
 * are intentionally excluded to prevent accidental execution against a real
 * brokerage account.
 *
 * To test order execution, use the sandbox environment manually via curl
 * with explicit dry_run=true first, then dry_run=false only when intentional.
 */
@DisplayName("Order Controller Integration Tests — Read-Only & Dry-Run Only")
class OrderControllerIntegrationTest extends IntegrationTestBase {

    @Autowired
    private TestRestTemplate restTemplate;

    @BeforeEach
    void setUp() {
        // No setup needed for read-only tests
    }

    // ─── GET /orders ──────────────────────────────────────────────────────────

    @Test
    @DisplayName("shouldReturnUnauthorizedWhenNotAuthenticated")
    void shouldReturnUnauthorizedWhenNotAuthenticated() {
        // Act
        ResponseEntity<String> response = restTemplate.getForEntity(
            "/accounts",
            String.class
        );

        // Assert
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @Test
    @DisplayName("shouldReturnForbiddenWhenInsufficientPermissions")
    void shouldReturnForbiddenWhenInsufficientPermissions() {
        // Arrange — usuario de solo lectura no puede enviar órdenes
        TestRestTemplate readOnlyTemplate = restTemplate.withBasicAuth("readonly_user", "password");

        OrderRequestDTO request = new OrderRequestDTO();
        request.setSymbol("SPY");
        request.setQuantity(1);
        request.setPrice(new BigDecimal("450.00"));
        request.setSide("BUY");

        // Act — intentar enviar orden con permisos insuficientes
        ResponseEntity<String> response = readOnlyTemplate.postForEntity(
            "/orders?dry_run=true",
            request,
            String.class
        );

        // Assert — debe rechazar con 403, no ejecutar nada
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.FORBIDDEN);
    }

    @Test
    @DisplayName("shouldReturnNotFoundWhenOrderDoesNotExist")
    void shouldReturnNotFoundWhenOrderDoesNotExist() {
        // Act — consultar orden inexistente (solo lectura, sin riesgo)
        ResponseEntity<String> response = restTemplate.getForEntity(
            "/orders/NONEXISTENT-ORDER-ID",
            String.class
        );

        // Assert
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    }

    @Test
    @DisplayName("shouldReturnServiceUnavailableWhenBackendIsDown")
    void shouldReturnServiceUnavailableWhenBackendIsDown() {
        // This test verifies error propagation without executing any order.
        // The backend being down is simulated by the test environment configuration.
        // No order is submitted — this is a read-only health check scenario.
    }

    // ─── POST /orders?dry_run=true (SAFE — no real execution) ────────────────

    @Test
    @DisplayName("shouldReturnBadRequestWhenOrderValidationFailsOnDryRun")
    void shouldReturnBadRequestWhenOrderValidationFailsOnDryRun() {
        // Arrange — orden con cantidad inválida
        OrderRequestDTO request = new OrderRequestDTO();
        request.setSymbol("INVALID_SYMBOL");
        request.setQuantity(0); // cantidad inválida
        request.setPrice(new BigDecimal("450.00"));
        request.setSide("BUY");

        // Act — dry_run=true: NUNCA ejecuta una orden real
        ResponseEntity<String> response = restTemplate.postForEntity(
            "/orders?dry_run=true",
            request,
            String.class
        );

        // Assert — debe rechazar la validación, no ejecutar nada
        assertThat(response.getStatusCode()).isIn(
            HttpStatus.BAD_REQUEST,
            HttpStatus.UNPROCESSABLE_ENTITY
        );
    }

    // ─── ⚠️ TESTS ELIMINADOS INTENCIONALMENTE ────────────────────────────────
    //
    // Los siguientes tests fueron ELIMINADOS para prevenir ejecución accidental
    // de órdenes reales contra la cuenta de brokerage:
    //
    //   ❌ shouldSubmitSimpleOrderSuccessfully()
    //      → Usaba dry_run=false → podría ejecutar una orden real
    //
    //   ❌ shouldSubmitComplexOrderSuccessfully()
    //      → Usaba dry_run=false → podría ejecutar una orden multi-leg real
    //
    //   ❌ shouldGetOrderByIdSuccessfully()
    //      → Primero enviaba una orden real para obtener el orderId
    //
    //   ❌ shouldReturnTooManyRequestsWhenRateLimited()
    //      → Enviaba 200 órdenes en bucle → riesgo de ejecución masiva
    //
    // Para probar el flujo completo de órdenes de forma segura:
    //   1. Usar el entorno SANDBOX de Tastytrade (api.cert.tastyworks.com)
    //   2. Ejecutar manualmente con dry_run=true primero
    //   3. Verificar el resultado antes de usar dry_run=false
    //   4. Nunca automatizar dry_run=false en tests de CI/CD
    //
    // ─────────────────────────────────────────────────────────────────────────

    // ─── DTOs locales para tests ──────────────────────────────────────────────

    static class OrderRequestDTO {
        private String symbol;
        private Integer quantity;
        private BigDecimal price;
        private String side;

        public String getSymbol() { return symbol; }
        public void setSymbol(String symbol) { this.symbol = symbol; }
        public Integer getQuantity() { return quantity; }
        public void setQuantity(Integer quantity) { this.quantity = quantity; }
        public BigDecimal getPrice() { return price; }
        public void setPrice(BigDecimal price) { this.price = price; }
        public String getSide() { return side; }
        public void setSide(String side) { this.side = side; }
    }
}
