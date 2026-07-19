package com.metradingplat.marketdata.adapter.authentication;

import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class SecurityContextTest {

    @Test
    void shouldCreateSecurityContextWithReadOnlyRole() {
        SecurityContext context = new SecurityContext("user123", SecurityContext.Role.READ_ONLY, "API_KEY");

        assertThat(context.getUserId()).isEqualTo("user123");
        assertThat(context.getRole()).isEqualTo(SecurityContext.Role.READ_ONLY);
        assertThat(context.getAuthenticationMethod()).isEqualTo("API_KEY");
    }

    @Test
    void shouldCreateSecurityContextWithReadWriteRole() {
        SecurityContext context = new SecurityContext("user123", SecurityContext.Role.READ_WRITE, "JWT");

        assertThat(context.getUserId()).isEqualTo("user123");
        assertThat(context.getRole()).isEqualTo(SecurityContext.Role.READ_WRITE);
        assertThat(context.getAuthenticationMethod()).isEqualTo("JWT");
    }

    @Test
    void shouldCheckReadPermissionForReadOnlyRole() {
        SecurityContext context = new SecurityContext("user123", SecurityContext.Role.READ_ONLY, "API_KEY");

        assertThat(context.hasPermission(SecurityContext.Permission.READ)).isTrue();
        assertThat(context.hasPermission(SecurityContext.Permission.WRITE)).isFalse();
    }

    @Test
    void shouldCheckBothPermissionsForReadWriteRole() {
        SecurityContext context = new SecurityContext("user123", SecurityContext.Role.READ_WRITE, "JWT");

        assertThat(context.hasPermission(SecurityContext.Permission.READ)).isTrue();
        assertThat(context.hasPermission(SecurityContext.Permission.WRITE)).isTrue();
    }

    @Test
    void shouldStoreClaims() {
        Map<String, Object> claims = new HashMap<>();
        claims.put("sub", "user123");
        claims.put("email", "user@example.com");

        SecurityContext context = new SecurityContext("user123", SecurityContext.Role.READ_WRITE, claims, "JWT");

        assertThat(context.getClaims()).containsEntry("sub", "user123");
        assertThat(context.getClaims()).containsEntry("email", "user@example.com");
    }

    @Test
    void shouldReturnFalseForPermissionWhenRoleIsNull() {
        SecurityContext context = new SecurityContext("user123", null, "API_KEY");

        assertThat(context.hasPermission(SecurityContext.Permission.READ)).isFalse();
        assertThat(context.hasPermission(SecurityContext.Permission.WRITE)).isFalse();
    }

    @Test
    void shouldSetAndGetRole() {
        SecurityContext context = new SecurityContext("user123", SecurityContext.Role.READ_ONLY, "API_KEY");
        context.setRole(SecurityContext.Role.READ_WRITE);

        assertThat(context.getRole()).isEqualTo(SecurityContext.Role.READ_WRITE);
    }
}
