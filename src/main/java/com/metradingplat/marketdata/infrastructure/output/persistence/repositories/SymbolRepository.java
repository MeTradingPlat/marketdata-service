package com.metradingplat.marketdata.infrastructure.output.persistence.repositories;

import java.util.Optional;
import org.springframework.data.jpa.repository.JpaRepository;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.SymbolEntity;

public interface SymbolRepository extends JpaRepository<SymbolEntity, Long> {
    Optional<SymbolEntity> findBySymbol(String symbol);
}
