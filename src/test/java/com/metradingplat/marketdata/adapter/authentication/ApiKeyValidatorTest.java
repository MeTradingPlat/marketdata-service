package com.metradingplat.marketdata.adapter.authentication;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class ApiKeyValidatorTest {

    @Test
    void shouldValidateCorrectApiKey() {
        ApiKeyValidator validator = new ApiKeyValidator("X-API-Key", "key1,key2,key3");

        assertThat(validator.isValidApiKey("key1")).isTrue();
        assertThat(validator.isValidApiKey("key2")).isTrue();
        assertThat(validator.isValidApiKey("key3")).isTrue();
    }

    @Test
    void shouldRejectInvalidApiKey() {
        ApiKeyValidator validator = new ApiKeyValidator("X-API-Key", "key1,key2,key3");

        assertThat(validator.isValidApiKey("invalid-key")).isFalse();
    }

    @Test
    void shouldRejectNullApiKey() {
        ApiKeyValidator validator = new ApiKeyValidator("X-API-Key", "key1,key2,key3");

        assertThat(validator.isValidApiKey(null)).isFalse();
    }

    @Test
    void shouldRejectEmptyApiKey() {
        ApiKeyValidator validator = new ApiKeyValidator("X-API-Key", "key1,key2,key3");

        assertThat(validator.isValidApiKey("")).isFalse();
    }

    @Test
    void shouldHandleEmptyConfiguration() {
        ApiKeyValidator validator = new ApiKeyValidator("X-API-Key", "");

        assertThat(validator.isValidApiKey("any-key")).isFalse();
    }

    @Test
    void shouldTrimWhitespaceFromKeys() {
        ApiKeyValidator validator = new ApiKeyValidator("X-API-Key", " key1 , key2 , key3 ");

        assertThat(validator.isValidApiKey("key1")).isTrue();
        assertThat(validator.isValidApiKey("key2")).isTrue();
        assertThat(validator.isValidApiKey("key3")).isTrue();
    }

    @Test
    void shouldReturnCorrectHeaderName() {
        ApiKeyValidator validator = new ApiKeyValidator("X-Custom-Key", "key1");

        assertThat(validator.getApiKeyHeader()).isEqualTo("X-Custom-Key");
    }
}
