package com.metradingplat.marketdata.infrastructure.output.persistence.repositories;

import java.util.List;

import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.CrudRepository;
import org.springframework.stereotype.Repository;

import com.metradingplat.marketdata.infrastructure.output.persistence.entities.SymbolFundamentalsEntity;

@Repository
public interface SpringSymbolFundamentalsRepository extends CrudRepository<SymbolFundamentalsEntity, String> {

    @Query("SELECT e FROM SymbolFundamentalsEntity e WHERE e.preMarketVolume IS NOT NULL "
            + "OR e.postMarketVolume IS NOT NULL OR e.preMarketClose IS NOT NULL OR e.postMarketClose IS NOT NULL")
    List<SymbolFundamentalsEntity> findAllWithExtendedHoursData();

    @Modifying
    @Query("UPDATE SymbolFundamentalsEntity e SET e.preMarketVolume = NULL, e.postMarketVolume = NULL, "
            + "e.preMarketClose = NULL, e.postMarketClose = NULL WHERE e.preMarketVolume IS NOT NULL "
            + "OR e.postMarketVolume IS NOT NULL OR e.preMarketClose IS NOT NULL OR e.postMarketClose IS NOT NULL")
    int clearExtendedHoursData();
}
