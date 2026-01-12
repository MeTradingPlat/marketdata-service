package com.metradingplat.marketdata.infrastructure.output.persistence.repositories;

import java.time.Instant;
import java.util.List;

import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.CandleEntity;

public interface CandleRepository extends JpaRepository<CandleEntity, Long> {

    @Query("SELECT c FROM CandleEntity c WHERE c.symbolObject.symbol = :symbol AND c.timeframe = :timeframe AND c.timestamp >= :from AND c.timestamp <= :to ORDER BY c.timestamp ASC")
    List<CandleEntity> findBySymbolAndTimeframeAndRange(
            @Param("symbol") String symbol,
            @Param("timeframe") EnumTimeframe timeframe,
            @Param("from") Instant from,
            @Param("to") Instant to);

    @Query("SELECT COUNT(c) FROM CandleEntity c WHERE c.symbolObject.symbol = :symbol AND c.timeframe = :timeframe AND c.timestamp >= :from AND c.timestamp <= :to")
    long countBySymbolAndTimeframeAndRange(
            @Param("symbol") String symbol,
            @Param("timeframe") EnumTimeframe timeframe,
            @Param("from") Instant from,
            @Param("to") Instant to);
}
