package com.metradingplat.marketdata.domain.models;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for SecurityContext value object.
 * Tests authentication, authorization, and role-based access control.
 */
@DisplayName("SecurityContext Tests")
class SecurityContextTest {

    @Test
    @DisplayName("shouldCreateUnauthenticatedContext")
    void shouldCreateUnauthenticatedContext() {
        // Arrange & Act
        SecurityContext context = SecurityContext.unauthenticated();

        // Assert
        assertFalse(context.isAuthenticated());
        assertNull(context.getUserId());
        assertNull(context.getUsername());
        assertNull(context.getRole());
    }

    @Test
    @DisplayName("shouldCreateAuthenticatedContext")
    void shouldCreateAuthenticatedContext() {
        // Arrange & Act
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_WRITE);

        // Assert
        assertTrue(context.isAuthenticated());
        assertEquals("user123", context.getUserId());
        assertEquals("john_doe", context.getUsername());
        assertEquals(Role.READ_WRITE, context.getRole());
    }

    @Test
    @DisplayName("shouldAllowReadForAuthenticatedUser")
    void shouldAllowReadForAuthenticatedUser() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_ONLY);

        // Act & Assert
        assertTrue(context.canRead());
    }

    @Test
    @DisplayName("shouldAllowWriteForReadWriteRole")
    void shouldAllowWriteForReadWriteRole() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_WRITE);

        // Act & Assert
        assertTrue(context.canWrite());
    }

    @Test
    @DisplayName("shouldDenyWriteForReadOnlyRole")
    void shouldDenyWriteForReadOnlyRole() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_ONLY);

        // Act & Assert
        assertFalse(context.canWrite());
    }

    @Test
    @DisplayName("shouldAllowWriteForApiClientRole")
    void shouldAllowWriteForApiClientRole() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("client123", "api_client", Role.API_CLIENT);

        // Act & Assert
        assertTrue(context.canWrite());
    }

    @Test
    @DisplayName("shouldIdentifyAdminRole")
    void shouldIdentifyAdminRole() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("admin123", "admin_user", Role.ADMIN);

        // Act & Assert
        assertTrue(context.isAdmin());
    }

    @Test
    @DisplayName("shouldNotIdentifyNonAdminAsAdmin")
    void shouldNotIdentifyNonAdminAsAdmin() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_WRITE);

        // Act & Assert
        assertFalse(context.isAdmin());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenAuthorizingUnauthenticatedUser")
    void shouldThrowExceptionWhenAuthorizingUnauthenticatedUser() {
        // Arrange
        SecurityContext context = SecurityContext.unauthenticated();

        // Act & Assert
        assertThrows(SecurityContext.SecurityException.class, () -> context.authorizeWrite());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenAuthorizingWithInsufficientRole")
    void shouldThrowExceptionWhenAuthorizingWithInsufficientRole() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_ONLY);

        // Act & Assert
        assertThrows(SecurityContext.SecurityException.class, () -> context.authorizeWrite());
    }

    @Test
    @DisplayName("shouldAuthorizeWriteForReadWriteRole")
    void shouldAuthorizeWriteForReadWriteRole() {
        // Arrange
        SecurityContext context = SecurityContext.authenticated("user123", "john_doe", Role.READ_WRITE);

        // Act & Assert
        assertDoesNotThrow(() -> context.authorizeWrite());
    }

    @Test
    @DisplayName("shouldAuthorizeReadForAllAuthenticatedUsers")
    void shouldAuthorizeReadForAllAuthenticatedUsers() {
        // Arrange
        SecurityContext readOnlyContext = SecurityContext.authenticated("user123", "john_doe", Role.READ_ONLY);
        SecurityContext readWriteContext = SecurityContext.authenticated("user456", "jane_doe", Role.READ_WRITE);

        // Act & Assert
        assertDoesNotThrow(() -> readOnlyContext.authorizeRead());
        assertDoesNotThrow(() -> readWriteContext.authorizeRead());
    }

    @Test
    @DisplayName("shouldThrowExceptionWhenReadingAsUnauthenticated")
    void shouldThrowExceptionWhenReadingAsUnauthenticated() {
        // Arrange
        SecurityContext context = SecurityContext.unauthenticated();

        // Act & Assert
        assertThrows(SecurityContext.SecurityException.class, () -> context.authorizeRead());
    }
}
