package com.metradingplat.marketdata.infrastructure.output.persistence.repositories;

import com.metradingplat.marketdata.infrastructure.output.persistence.entities.SymbolFundamentalsEntity;
import org.springframework.data.repository.CrudRepository;
import org.springframework.stereotype.Repository;

@Repository
public interface SpringSymbolFundamentalsRepository extends CrudRepository<SymbolFundamentalsEntity, String> {
}
