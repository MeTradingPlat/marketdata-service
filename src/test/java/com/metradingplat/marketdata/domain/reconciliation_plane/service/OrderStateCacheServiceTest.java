package com.metradingplat.marketdata.domain.reconciliation_plane.service;

import com.metradingplat.marketdata.domain.reconciliation_plane.entity.OrderState;
import com.metradingplat.marketdata.domain.reconciliation_plane.port.OrderStateRepositoryPort;
import com.metradingplat.marketdata.domain.control_plane.port.TastytradeFacadePort;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class OrderStateCacheServiceTest {
    @Mock
    private OrderStateRepositoryPort orderStateRepository;
    
    @Mock
    private TastytradeFacadePort tastytradeFacade;
    
    private OrderStateCacheService cacheService;
    private OrderState testOrderState;

    @BeforeEach
    void setUp() {
        cacheService = new OrderStateCacheService(orderStateRepository, tastytradeFacade, 30000);
        
        testOrderState = new OrderState();
        testOrderState.setOrderId("ORD123");
        testOrderState.setStatus("PENDING");
    }

    @Test
    void shouldUpdateOrderStateFromEvent() {
        cacheService.updateFromEvent(testOrderState);
        
        Optional<OrderState> cached = cacheService.getOrderState("ORD123");
        assertThat(cached).isPresent();
        assertThat(cached.get().getOrderId()).isEqualTo("ORD123");
        assertThat(cached.get().getStatus()).isEqualTo("PENDING");
    }

    @Test
    void shouldReturnCachedOrderStateOnHit() {
        cacheService.updateFromEvent(testOrderState);
        
        Optional<OrderState> first = cacheService.getOrderState("ORD123");
        Optional<OrderState> second = cacheService.getOrderState("ORD123");
        
        assertThat(first).isPresent();
        assertThat(second).isPresent();
        assertThat(cacheService.getCacheHits()).isGreaterThanOrEqualTo(1);
    }

    @Test
    void shouldReturnEmptyOnCacheMiss() {
        Optional<OrderState> result = cacheService.getOrderState("NONEXISTENT");
        
        assertThat(result).isEmpty();
        assertThat(cacheService.getCacheMisses()).isGreaterThan(0);
    }

    @Test
    void shouldInvalidateSingleOrderState() {
        cacheService.updateFromEvent(testOrderState);
        assertThat(cacheService.getCacheSize()).isEqualTo(1);
        
        cacheService.invalidate("ORD123");
        
        assertThat(cacheService.getCacheSize()).isEqualTo(0);
    }

    @Test
    void shouldInvalidateAllOrderStates() {
        OrderState order1 = new OrderState();
        order1.setOrderId("ORD1");
        
        OrderState order2 = new OrderState();
        order2.setOrderId("ORD2");
        
        cacheService.updateFromEvent(order1);
        cacheService.updateFromEvent(order2);
        assertThat(cacheService.getCacheSize()).isEqualTo(2);
        
        cacheService.invalidateAll();
        
        assertThat(cacheService.getCacheSize()).isEqualTo(0);
    }

    @Test
    void shouldCalculateCacheHitRate() {
        cacheService.updateFromEvent(testOrderState);
        
        cacheService.getOrderState("ORD123");
        cacheService.getOrderState("ORD123");
        cacheService.getOrderState("NONEXISTENT");
        
        long hitRate = cacheService.getCacheHitRate();
        assertThat(hitRate).isGreaterThan(0);
        assertThat(hitRate).isLessThanOrEqualTo(100);
    }

    @Test
    void shouldTrackCacheSize() {
        OrderState order1 = new OrderState();
        order1.setOrderId("ORD1");
        
        OrderState order2 = new OrderState();
        order2.setOrderId("ORD2");
        
        cacheService.updateFromEvent(order1);
        assertThat(cacheService.getCacheSize()).isEqualTo(1);
        
        cacheService.updateFromEvent(order2);
        assertThat(cacheService.getCacheSize()).isEqualTo(2);
    }
}
