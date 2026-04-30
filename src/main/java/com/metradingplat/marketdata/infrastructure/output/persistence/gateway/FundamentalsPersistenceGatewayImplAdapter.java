package com.metradingplat.marketdata.infrastructure.output.persistence.gateway;

import com.metradingplat.marketdata.application.output.FundamentalsPersistenceGatewayIntPort;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.output.persistence.entities.SymbolFundamentalsEntity;
import com.metradingplat.marketdata.infrastructure.output.persistence.mappers.SymbolFundamentalsMapper;
import com.metradingplat.marketdata.infrastructure.output.persistence.repositories.SpringSymbolFundamentalsRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Optional;

@Component
@RequiredArgsConstructor
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
}
