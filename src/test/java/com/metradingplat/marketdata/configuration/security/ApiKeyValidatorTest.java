package com.metradingplat.marketdata.configuration.security;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for ApiKeyValidator.
 * Tests API key validation logic.
 */
@DisplayName("ApiKeyValidator Tests")
class ApiKeyValidatorTest {

    private ApiKeyValidator validator;

    @BeforeEach
    void setUp() {
        String apiKeysConfig = "client1:secret1,client2:secret2,client3:secret3";
        validator = new ApiKeyValidator(apiKeysConfig);
    }

    @Test
    @DisplayName("shouldValidateCorrectApiKey")
    void shouldValidateCorrectApiKey() {
        // Arrange
        String apiKey = "client1:secret1";

        // Act
        String clientId = validator.validateApiKey(apiKey);

        // Assert
        assertEquals("client1", clientId);
    }

    @Test
    @DisplayName("shouldRejectInvalidApiKey")
    void shouldRejectInvalidApiKey() {
        // Arrange
        String apiKey = "client1:wrongsecret";

        // Act
        String clientId = validator.validateApiKey(apiKey);

        // Assert
        assertNull(clientId);
    }

    @Test
    @DisplayName("shouldRejectNullApiKey")
    void shouldRejectNullApiKey() {
        // Arrange
        String apiKey = null;

        // Act
        String clientId = validator.validateApiKey(apiKey);

        // Assert
        assertNull(clientId);
    }

    @Test
    @DisplayName("shouldRejectEmptyApiKey")
    void shouldRejectEmptyApiKey() {
        // Arrange
        String apiKey = "";

        // Act
        String clientId = validator.validateApiKey(apiKey);

        // Assert
        assertNull(clientId);
    }

    @Test
    @DisplayName("shouldRejectMalformedApiKey")
    void shouldRejectMalformedApiKey() {
        // Arrange
        String apiKey = "malformed_key_without_colon";

        // Act
        String clientId = validator.validateApiKey(apiKey);

        // Assert
        assertNull(clientId);
    }

    @Test
    @DisplayName("shouldValidateMultipleApiKeys")
    void shouldValidateMultipleApiKeys() {
        // Arrange & Act & Assert
        assertEquals("client1", validator.validateApiKey("client1:secret1"));
        assertEquals("client2", validator.validateApiKey("client2:secret2"));
        assertEquals("client3", validator.validateApiKey("client3:secret3"));
    }

    @Test
    @DisplayName("shouldReturnTrueForValidApiKey")
    void shouldReturnTrueForValidApiKey() {
        // Arrange
        String apiKey = "client1:secret1";

        // Act
        boolean isValid = validator.isValidApiKey(apiKey);

        // Assert
        assertTrue(isValid);
    }

    @Test
    @DisplayName("shouldReturnFalseForInvalidApiKey")
    void shouldReturnFalseForInvalidApiKey() {
        // Arrange
        String apiKey = "client1:wrongsecret";

        // Act
        boolean isValid = validator.isValidApiKey(apiKey);

        // Assert
        assertFalse(isValid);
    }

    @Test
    @DisplayName("shouldHandleEmptyConfiguration")
    void shouldHandleEmptyConfiguration() {
        // Arrange
        ApiKeyValidator emptyValidator = new ApiKeyValidator("");

        // Act
        String clientId = emptyValidator.validateApiKey("client1:secret1");

        // Assert
        assertNull(clientId);
    }

    @Test
    @DisplayName("shouldHandleNullConfiguration")
    void shouldHandleNullConfiguration() {
        // Arrange
        ApiKeyValidator nullValidator = new ApiKeyValidator(null);

        // Act
        String clientId = nullValidator.validateApiKey("client1:secret1");

        // Assert
        assertNull(clientId);
    }
}
