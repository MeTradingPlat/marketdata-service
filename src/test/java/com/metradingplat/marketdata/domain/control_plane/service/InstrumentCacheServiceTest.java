package com.metradingplat.marketdata.domain.control_plane.service;

import com.metradingplat.marketdata.domain.control_plane.entity.Instrument;
import com.metradingplat.marketdata.domain.control_plane.port.InstrumentRepositoryPort;
import com.metradingplat.marketdata.domain.control_plane.port.TastytradeFacadePort;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.time.LocalDate;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class InstrumentCacheServiceTest {
    @Mock
    private InstrumentRepositoryPort instrumentRepository;
    
    @Mock
    private TastytradeFacadePort tastytradeFacade;
    
    private InstrumentCacheService cacheService;
    private Instrument testInstrument;

    @BeforeEach
    void setUp() {
        cacheService = new InstrumentCacheService(instrumentRepository, tastytradeFacade, 3600000);
        
        testInstrument = new Instrument();
        testInstrument.setSymbol("SPY");
        testInstrument.setMultiplier(BigDecimal.ONE);
    }

    @Test
    void shouldLoadInstrumentsFromDatabaseOnInitialize() {
        when(instrumentRepository.findAll()).thenReturn(List.of(testInstrument));
        
        cacheService.initialize();
        
        Optional<Instrument> cached = cacheService.getBySymbol("SPY");
        assertThat(cached).isPresent();
        assertThat(cached.get().getSymbol()).isEqualTo("SPY");
    }

    @Test
    void shouldReturnCachedInstrumentOnHit() {
        when(instrumentRepository.findAll()).thenReturn(List.of(testInstrument));
        cacheService.initialize();
        
        Optional<Instrument> first = cacheService.getBySymbol("SPY");
        Optional<Instrument> second = cacheService.getBySymbol("SPY");
        
        assertThat(first).isPresent();
        assertThat(second).isPresent();
        assertThat(cacheService.getCacheHits()).isGreaterThanOrEqualTo(1);
    }

    @Test
    void shouldReturnEmptyOnCacheMiss() {
        when(instrumentRepository.findAll()).thenReturn(List.of());
        cacheService.initialize();
        
        Optional<Instrument> result = cacheService.getBySymbol("NONEXISTENT");
        
        assertThat(result).isEmpty();
        assertThat(cacheService.getCacheMisses()).isGreaterThan(0);
    }

    @Test
    void shouldCalculateCacheHitRate() {
        when(instrumentRepository.findAll()).thenReturn(List.of(testInstrument));
        cacheService.initialize();
        
        cacheService.getBySymbol("SPY");
        cacheService.getBySymbol("SPY");
        cacheService.getBySymbol("NONEXISTENT");
        
        long hitRate = cacheService.getCacheHitRate();
        assertThat(hitRate).isGreaterThan(0);
        assertThat(hitRate).isLessThanOrEqualTo(100);
    }

    @Test
    void shouldReturnAllCachedInstruments() {
        Instrument spy = new Instrument();
        spy.setSymbol("SPY");
        
        Instrument qqq = new Instrument();
        qqq.setSymbol("QQQ");
        
        when(instrumentRepository.findAll()).thenReturn(List.of(spy, qqq));
        cacheService.initialize();
        
        List<Instrument> all = cacheService.getAll();
        
        assertThat(all).hasSize(2);
        assertThat(all).extracting(Instrument::getSymbol).containsExactlyInAnyOrder("SPY", "QQQ");
    }
}
