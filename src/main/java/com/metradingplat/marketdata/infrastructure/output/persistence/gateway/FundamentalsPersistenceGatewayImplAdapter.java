package com.metradingplat.marketdata.infrastructure.output.persistence.gateway;

import com.metradingplat.marketdata.application.output.FundamentalsPersistenceGatewayIntPort;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.output.persistence.entities.SymbolFundamentalsEntity;
import com.metradingplat.marketdata.infrastructure.output.persistence.mappers.SymbolFundamentalsMapper;
import com.metradingplat.marketdata.infrastructure.output.persistence.repositories.SpringSymbolFundamentalsRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

@Component
@RequiredArgsConstructor
@Slf4j
public class FundamentalsPersistenceGatewayImplAdapter implements FundamentalsPersistenceGatewayIntPort {

    private final SpringSymbolFundamentalsRepository repository;
    private final SymbolFundamentalsMapper mapper;

    @Override
    public Optional<FundamentalData> findBySymbol(String symbol) {
        return repository.findById(symbol).map(mapper::toDomain);
    }

    @Override
    public FundamentalData save(FundamentalData fundamentalData) {
        SymbolFundamentalsEntity entity = mapper.toEntity(fundamentalData);
        entity.setLastUpdated(Instant.now());
        SymbolFundamentalsEntity savedEntity = repository.save(entity);
        return mapper.toDomain(savedEntity);
    }

    @Override
    public List<FundamentalData> findAllWithExtendedHoursData() {
        return repository.findAllWithExtendedHoursData().stream().map(mapper::toDomain).toList();
    }

    @Override
    @Transactional
    public void clearExtendedHoursData() {
        int cleared = repository.clearExtendedHoursData();
        log.info("DB reset: cleared pre/post-market volume+close for {} symbols", cleared);
    }
}
