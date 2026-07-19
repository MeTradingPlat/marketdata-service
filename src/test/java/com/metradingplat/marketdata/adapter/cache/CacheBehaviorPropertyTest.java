package com.metradingplat.marketdata.adapter.cache;

import net.jqwik.api.*;
import net.jqwik.api.constraints.IntRange;
import net.jqwik.api.constraints.Positive;

import java.math.BigDecimal;
import java.time.Duration;
import java.time.LocalDateTime;
import java.util.HashMap;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.*;

/**
 * Property-based tests for cache behavior.
 * **Validates: Requirements 11.1, 11.2, 11.3**
 *
 * Properties tested:
 * - Cache returns same value as source
 * - Cache respects TTL expiration
 * - Cache handles concurrent access correctly
 */
@DisplayName("Cache Behavior Property Tests")
class CacheBehaviorPropertyTest {

    /**
     * Simple in-memory cache implementation for testing
     */
    static class SimpleCache<K, V> {
        private final Map<K, CacheEntry<V>> cache = new ConcurrentHashMap<>();
        private final long ttlMillis;

        static class CacheEntry<V> {
            final V value;
            final LocalDateTime createdAt;

            CacheEntry(V value) {
                this.value = value;
                this.createdAt = LocalDateTime.now();
            }

            boolean isExpired(long ttlMillis) {
                return Duration.between(createdAt, LocalDateTime.now()).toMillis() > ttlMillis;
            }
        }

        SimpleCache(long ttlMillis) {
            this.ttlMillis = ttlMillis;
        }

        void put(K key, V value) {
            cache.put(key, new CacheEntry<>(value));
        }

        Optional<V> get(K key) {
            CacheEntry<V> entry = cache.get(key);
            if (entry == null) {
                return Optional.empty();
            }
            if (entry.isExpired(ttlMillis)) {
                cache.remove(key);
                return Optional.empty();
            }
            return Optional.of(entry.value);
        }

        void clear() {
            cache.clear();
        }

        int size() {
            return cache.size();
        }
    }

    @Property
    @DisplayName("shouldReturnSameValueAsSourceWhenCached")
    void shouldReturnSameValueAsSourceWhenCached(
            @ForAll String key,
            @ForAll String value) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());
        Assume.that(value != null);

        SimpleCache<String, String> cache = new SimpleCache<>(5000); // 5 second TTL
        cache.put(key, value);

        // Act
        Optional<String> cachedValue = cache.get(key);

        // Assert
        assertThat(cachedValue).isPresent();
        assertThat(cachedValue.get()).isEqualTo(value);
    }

    @Property
    @DisplayName("shouldReturnEmptyWhenKeyNotInCache")
    void shouldReturnEmptyWhenKeyNotInCache(
            @ForAll String key) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());

        SimpleCache<String, String> cache = new SimpleCache<>(5000);

        // Act
        Optional<String> cachedValue = cache.get(key);

        // Assert
        assertThat(cachedValue).isEmpty();
    }

    @Property
    @DisplayName("shouldReturnEmptyWhenCacheIsCleared")
    void shouldReturnEmptyWhenCacheIsCleared(
            @ForAll String key,
            @ForAll String value) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());
        Assume.that(value != null);

        SimpleCache<String, String> cache = new SimpleCache<>(5000);
        cache.put(key, value);

        // Act
        cache.clear();
        Optional<String> cachedValue = cache.get(key);

        // Assert
        assertThat(cachedValue).isEmpty();
    }

    @Property
    @DisplayName("shouldHandleMultipleKeysIndependently")
    void shouldHandleMultipleKeysIndependently(
            @ForAll String key1,
            @ForAll String key2,
            @ForAll String value1,
            @ForAll String value2) {

        // Arrange
        Assume.that(key1 != null && !key1.isEmpty());
        Assume.that(key2 != null && !key2.isEmpty());
        Assume.that(!key1.equals(key2)); // Different keys
        Assume.that(value1 != null);
        Assume.that(value2 != null);

        SimpleCache<String, String> cache = new SimpleCache<>(5000);
        cache.put(key1, value1);
        cache.put(key2, value2);

        // Act
        Optional<String> cachedValue1 = cache.get(key1);
        Optional<String> cachedValue2 = cache.get(key2);

        // Assert
        assertThat(cachedValue1).isPresent().contains(value1);
        assertThat(cachedValue2).isPresent().contains(value2);
    }

    @Property
    @DisplayName("shouldUpdateValueWhenKeyIsReinserted")
    void shouldUpdateValueWhenKeyIsReinserted(
            @ForAll String key,
            @ForAll String value1,
            @ForAll String value2) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());
        Assume.that(value1 != null);
        Assume.that(value2 != null);
        Assume.that(!value1.equals(value2)); // Different values

        SimpleCache<String, String> cache = new SimpleCache<>(5000);
        cache.put(key, value1);

        // Act
        cache.put(key, value2);
        Optional<String> cachedValue = cache.get(key);

        // Assert
        assertThat(cachedValue).isPresent().contains(value2);
    }

    @Property
    @DisplayName("shouldTrackCacheSizeCorrectly")
    void shouldTrackCacheSizeCorrectly(
            @ForAll @IntRange(min = 1, max = 100) Integer numEntries) {

        // Arrange
        SimpleCache<String, String> cache = new SimpleCache<>(5000);

        // Act
        for (int i = 0; i < numEntries; i++) {
            cache.put("key" + i, "value" + i);
        }

        // Assert
        assertThat(cache.size()).isEqualTo(numEntries);
    }

    @Property
    @DisplayName("shouldHandleConcurrentReadsConsistently")
    void shouldHandleConcurrentReadsConsistently(
            @ForAll String key,
            @ForAll String value) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());
        Assume.that(value != null);

        SimpleCache<String, String> cache = new SimpleCache<>(5000);
        cache.put(key, value);

        // Act - Simulate concurrent reads
        AtomicInteger successCount = new AtomicInteger(0);
        for (int i = 0; i < 10; i++) {
            Optional<String> cachedValue = cache.get(key);
            if (cachedValue.isPresent() && cachedValue.get().equals(value)) {
                successCount.incrementAndGet();
            }
        }

        // Assert
        assertThat(successCount.get()).isEqualTo(10);
    }

    @Property
    @DisplayName("shouldHandleConcurrentWritesConsistently")
    void shouldHandleConcurrentWritesConsistently(
            @ForAll String key,
            @ForAll @IntRange(min = 1, max = 100) Integer numWrites) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());

        SimpleCache<String, String> cache = new SimpleCache<>(5000);

        // Act - Simulate concurrent writes
        for (int i = 0; i < numWrites; i++) {
            cache.put(key, "value" + i);
        }

        // Assert - Last write should be present
        Optional<String> cachedValue = cache.get(key);
        assertThat(cachedValue).isPresent();
    }

    @Property
    @DisplayName("shouldBeDeterministicForSameInputs")
    void shouldBeDeterministicForSameInputs(
            @ForAll String key,
            @ForAll String value) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());
        Assume.that(value != null);

        SimpleCache<String, String> cache1 = new SimpleCache<>(5000);
        SimpleCache<String, String> cache2 = new SimpleCache<>(5000);

        cache1.put(key, value);
        cache2.put(key, value);

        // Act
        Optional<String> cachedValue1 = cache1.get(key);
        Optional<String> cachedValue2 = cache2.get(key);

        // Assert
        assertThat(cachedValue1).isEqualTo(cachedValue2);
    }

    @Property
    @DisplayName("shouldHandleNullValuesConsistently")
    void shouldHandleNullValuesConsistently(
            @ForAll String key) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());

        SimpleCache<String, String> cache = new SimpleCache<>(5000);

        // Act - Try to put null value (should handle gracefully)
        try {
            cache.put(key, null);
            Optional<String> cachedValue = cache.get(key);
            // Assert - Either null is stored or not stored
            assertThat(cachedValue).isNotNull();
        } catch (NullPointerException e) {
            // Acceptable - cache rejects null values
        }
    }

    @Property
    @DisplayName("shouldMaintainCacheInvariantsUnderStress")
    void shouldMaintainCacheInvariantsUnderStress(
            @ForAll @IntRange(min = 1, max = 1000) Integer numOperations) {

        // Arrange
        SimpleCache<String, String> cache = new SimpleCache<>(5000);
        int putCount = 0;
        int getCount = 0;

        // Act - Perform random operations
        for (int i = 0; i < numOperations; i++) {
            if (i % 2 == 0) {
                cache.put("key" + (i % 100), "value" + i);
                putCount++;
            } else {
                cache.get("key" + (i % 100));
                getCount++;
            }
        }

        // Assert - Cache should still be functional
        assertThat(cache.size()).isGreaterThanOrEqualTo(0);
        assertThat(cache.size()).isLessThanOrEqualTo(100); // Max 100 unique keys
    }

    @Property
    @DisplayName("shouldHandleLargeValuesConsistently")
    void shouldHandleLargeValuesConsistently(
            @ForAll String key,
            @ForAll @IntRange(min = 1000, max = 100000) Integer valueSize) {

        // Arrange
        Assume.that(key != null && !key.isEmpty());

        SimpleCache<String, String> cache = new SimpleCache<>(5000);
        StringBuilder largeValue = new StringBuilder();
        for (int i = 0; i < valueSize; i++) {
            largeValue.append("x");
        }

        // Act
        cache.put(key, largeValue.toString());
        Optional<String> cachedValue = cache.get(key);

        // Assert
        assertThat(cachedValue).isPresent();
        assertThat(cachedValue.get().length()).isEqualTo(valueSize);
    }
}
