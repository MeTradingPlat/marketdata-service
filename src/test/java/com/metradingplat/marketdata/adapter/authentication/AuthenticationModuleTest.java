package com.metradingplat.marketdata.adapter.authentication;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.client.RestTemplate;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AuthenticationModuleTest {

    @Mock
    private RestTemplate restTemplate;

    private AuthenticationModule authenticationModule;

    @BeforeEach
    void setUp() {
        authenticationModule = new AuthenticationModule(
                restTemplate,
                "https://cert.tastyworks.com",
                "/oauth/token",
                "test-client-id",
                "test-client-secret",
                86400,
                300,
                3,
                1000,
                2.0,
                8000
        );
    }

    @Test
    void shouldAcquireTokenOnInitialization() {
        AuthenticationModule.OAuthTokenResponse tokenResponse = new AuthenticationModule.OAuthTokenResponse();
        tokenResponse.setAccess_token("test-token-123");
        tokenResponse.setToken_type("Bearer");
        tokenResponse.setExpires_in(86400);

        when(restTemplate.postForEntity(
                anyString(),
                org.mockito.ArgumentMatchers.any(HttpEntity.class),
                eq(AuthenticationModule.OAuthTokenResponse.class)
        )).thenReturn(new ResponseEntity<>(tokenResponse, HttpStatus.OK));

        authenticationModule.initialize();

        String token = authenticationModule.getAccessToken();
        assertThat(token).isEqualTo("test-token-123");
    }

    @Test
    void shouldReturnValidTokenWhenNotExpired() {
        AuthenticationModule.OAuthTokenResponse tokenResponse = new AuthenticationModule.OAuthTokenResponse();
        tokenResponse.setAccess_token("test-token-123");
        tokenResponse.setToken_type("Bearer");
        tokenResponse.setExpires_in(86400);

        when(restTemplate.postForEntity(
                anyString(),
                org.mockito.ArgumentMatchers.any(HttpEntity.class),
                eq(AuthenticationModule.OAuthTokenResponse.class)
        )).thenReturn(new ResponseEntity<>(tokenResponse, HttpStatus.OK));

        authenticationModule.initialize();
        String token1 = authenticationModule.getAccessToken();
        String token2 = authenticationModule.getAccessToken();

        assertThat(token1).isEqualTo(token2);
        assertThat(token1).isEqualTo("test-token-123");
    }

    @Test
    void shouldThrowExceptionWhenTokenAcquisitionFails() {
        when(restTemplate.postForEntity(
                anyString(),
                org.mockito.ArgumentMatchers.any(HttpEntity.class),
                eq(AuthenticationModule.OAuthTokenResponse.class)
        )).thenThrow(new RuntimeException("Connection failed"));

        assertThatThrownBy(() -> authenticationModule.initialize())
                .isInstanceOf(RuntimeException.class)
                .hasMessageContaining("Failed to initialize authentication module");
    }

    @Test
    void shouldRetryTokenRefreshOnFailure() {
        AuthenticationModule.OAuthTokenResponse tokenResponse = new AuthenticationModule.OAuthTokenResponse();
        tokenResponse.setAccess_token("test-token-123");
        tokenResponse.setToken_type("Bearer");
        tokenResponse.setExpires_in(86400);

        when(restTemplate.postForEntity(
                anyString(),
                org.mockito.ArgumentMatchers.any(HttpEntity.class),
                eq(AuthenticationModule.OAuthTokenResponse.class)
        ))
                .thenThrow(new RuntimeException("Temporary failure"))
                .thenThrow(new RuntimeException("Temporary failure"))
                .thenReturn(new ResponseEntity<>(tokenResponse, HttpStatus.OK));

        authenticationModule.initialize();

        String token = authenticationModule.getAccessToken();
        assertThat(token).isEqualTo("test-token-123");
    }
}
