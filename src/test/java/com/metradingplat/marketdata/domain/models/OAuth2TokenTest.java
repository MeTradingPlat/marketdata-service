package com.metradingplat.marketdata.domain.models;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.time.Instant;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for OAuth2Token value object.
 * Tests token expiration and refresh logic.
 */
@DisplayName("OAuth2Token Tests")
class OAuth2TokenTest {

    private OAuth2Token token;

    @BeforeEach
    void setUp() {
        // Create token with 1 hour TTL
        token = new OAuth2Token("test_access_token", "Bearer", 3600);
    }

    @Test
    @DisplayName("shouldCreateTokenWithCorrectValues")
    void shouldCreateTokenWithCorrectValues() {
        // Arrange & Act & Assert
        assertEquals("test_access_token", token.getAccessToken());
        assertEquals("Bearer", token.getTokenType());
        assertNotNull(token.getExpiresAt());
        assertNotNull(token.getIssuedAt());
    }

    @Test
    @DisplayName("shouldNotBeExpiredImmediatelyAfterCreation")
    void shouldNotBeExpiredImmediatelyAfterCreation() {
        // Arrange & Act & Assert
        assertFalse(token.isExpired());
    }

    @Test
    @DisplayName("shouldBeExpiredAfterTtlPasses")
    void shouldBeExpiredAfterTtlPasses() {
        // Arrange
        OAuth2Token expiredToken = new OAuth2Token("test_token", "Bearer", -1);

        // Act & Assert
        assertTrue(expiredToken.isExpired());
    }

    @Test
    @DisplayName("shouldShouldRefreshBeforeExpiration")
    void shouldShouldRefreshBeforeExpiration() {
        // Arrange
        OAuth2Token shortLivedToken = new OAuth2Token("test_token", "Bearer", 10);

        // Act
        boolean shouldRefresh = shortLivedToken.shouldRefresh(5);

        // Assert
        assertTrue(shouldRefresh);
    }

    @Test
    @DisplayName("shouldNotRefreshIfNotNearExpiration")
    void shouldNotRefreshIfNotNearExpiration() {
        // Arrange
        OAuth2Token longLivedToken = new OAuth2Token("test_token", "Bearer", 3600);

        // Act
        boolean shouldRefresh = longLivedToken.shouldRefresh(30);

        // Assert
        assertFalse(shouldRefresh);
    }

    @Test
    @DisplayName("shouldGenerateCorrectAuthorizationHeader")
    void shouldGenerateCorrectAuthorizationHeader() {
        // Arrange & Act
        String header = token.getAuthorizationHeader();

        // Assert
        assertEquals("Bearer test_access_token", header);
    }

    @Test
    @DisplayName("shouldHandleCustomTokenType")
    void shouldHandleCustomTokenType() {
        // Arrange
        OAuth2Token customToken = new OAuth2Token("test_token", "Custom", 3600);

        // Act
        String header = customToken.getAuthorizationHeader();

        // Assert
        assertEquals("Custom test_token", header);
    }

    @Test
    @DisplayName("shouldHaveCorrectExpirationTime")
    void shouldHaveCorrectExpirationTime() {
        // Arrange
        long ttlSeconds = 3600;
        OAuth2Token testToken = new OAuth2Token("test_token", "Bearer", ttlSeconds);

        // Act
        Instant issuedAt = testToken.getIssuedAt();
        Instant expiresAt = testToken.getExpiresAt();

        // Assert
        assertTrue(expiresAt.isAfter(issuedAt));
        long actualTtl = expiresAt.getEpochSecond() - issuedAt.getEpochSecond();
        assertEquals(ttlSeconds, actualTtl);
    }
}
