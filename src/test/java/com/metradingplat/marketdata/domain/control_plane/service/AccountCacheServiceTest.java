package com.metradingplat.marketdata.domain.control_plane.service;

import com.metradingplat.marketdata.domain.control_plane.entity.Account;
import com.metradingplat.marketdata.domain.control_plane.port.AccountRepositoryPort;
import com.metradingplat.marketdata.domain.control_plane.port.TastytradeFacadePort;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.math.BigDecimal;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AccountCacheServiceTest {
    @Mock
    private AccountRepositoryPort accountRepository;
    
    @Mock
    private TastytradeFacadePort tastytradeFacade;
    
    private AccountCacheService cacheService;
    private Account testAccount;

    @BeforeEach
    void setUp() {
        cacheService = new AccountCacheService(accountRepository, tastytradeFacade, 300000);
        
        testAccount = new Account();
        testAccount.setAccountId("ACC123");
        testAccount.setCashBalance(BigDecimal.valueOf(10000));
        testAccount.setBuyingPower(BigDecimal.valueOf(20000));
    }

    @Test
    void shouldLoadAccountFromDatabaseOnInitialize() {
        when(accountRepository.findAll()).thenReturn(List.of(testAccount));
        
        cacheService.initialize();
        
        Optional<Account> cached = cacheService.getAccount();
        assertThat(cached).isPresent();
        assertThat(cached.get().getAccountId()).isEqualTo("ACC123");
    }

    @Test
    void shouldUpdateAccountFromEvent() {
        when(accountRepository.findAll()).thenReturn(List.of(testAccount));
        cacheService.initialize();
        
        Account updatedAccount = new Account();
        updatedAccount.setAccountId("ACC123");
        updatedAccount.setCashBalance(BigDecimal.valueOf(15000));
        
        cacheService.updateFromEvent(updatedAccount);
        
        Optional<Account> cached = cacheService.getAccount();
        assertThat(cached).isPresent();
        assertThat(cached.get().getCashBalance()).isEqualTo(BigDecimal.valueOf(15000));
    }

    @Test
    void shouldReturnCachedAccountOnHit() {
        when(accountRepository.findAll()).thenReturn(List.of(testAccount));
        cacheService.initialize();
        
        Optional<Account> first = cacheService.getAccount();
        Optional<Account> second = cacheService.getAccount();
        
        assertThat(first).isPresent();
        assertThat(second).isPresent();
        assertThat(cacheService.getCacheHits()).isGreaterThanOrEqualTo(1);
    }

    @Test
    void shouldReturnEmptyWhenNoAccountCached() {
        when(accountRepository.findAll()).thenReturn(List.of());
        cacheService.initialize();
        
        Optional<Account> result = cacheService.getAccount();
        
        assertThat(result).isEmpty();
        assertThat(cacheService.getCacheMisses()).isGreaterThan(0);
    }

    @Test
    void shouldCalculateCacheHitRate() {
        when(accountRepository.findAll()).thenReturn(List.of(testAccount));
        cacheService.initialize();
        
        cacheService.getAccount();
        cacheService.getAccount();
        
        long hitRate = cacheService.getCacheHitRate();
        assertThat(hitRate).isGreaterThan(0);
        assertThat(hitRate).isLessThanOrEqualTo(100);
    }
}
