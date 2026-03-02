package com.metradingplat.marketdata.infrastructure.output.persistence.repositories;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.infrastructure.output.persistence.entities.CandleEntity;
import org.springframework.data.domain.Pageable;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.List;

@Repository
public interface CandleRepository extends JpaRepository<CandleEntity, Long> {

        CandleEntity findTopBySymbolAndTimeframeOrderByTimestampDesc(String symbol, EnumTimeframe timeframe);

        CandleEntity findTopBySymbolAndTimeframeOrderByTimestampAsc(String symbol, EnumTimeframe timeframe);

        @Query("SELECT c FROM CandleEntity c WHERE c.symbol = :symbol AND c.timeframe = :timeframe ORDER BY c.timestamp DESC")
        List<CandleEntity> findLatestCandles(@Param("symbol") String symbol,
                        @Param("timeframe") EnumTimeframe timeframe,
                        Pageable pageable);

        @Query("SELECT c FROM CandleEntity c WHERE c.symbol = :symbol AND c.timeframe = :timeframe AND c.timestamp >= :fromTime ORDER BY c.timestamp ASC")
        List<CandleEntity> findCandlesFrom(@Param("symbol") String symbol, @Param("timeframe") EnumTimeframe timeframe,
                        @Param("fromTime") Instant fromTime);

        @Modifying
        @Transactional
        @Query("DELETE FROM CandleEntity c WHERE c.timeframe NOT IN :timeframes")
        void deleteByTimeframeNotIn(@Param("timeframes") List<EnumTimeframe> timeframes);

        @Modifying
        @Transactional
        @Query(value = "INSERT INTO candles_historicas (symbol, timeframe, timestamp, open, high, low, close, volume) "
                        +
                        "VALUES (:#{#c.symbol}, :#{#c.timeframe.name()}, :#{#c.timestamp}, :#{#c.open}, :#{#c.high}, :#{#c.low}, :#{#c.close}, :#{#c.volume}) "
                        +
                        "ON CONFLICT (symbol, timeframe, timestamp) DO UPDATE SET " +
                        "open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low, close = EXCLUDED.close, volume = EXCLUDED.volume", nativeQuery = true)
        void upsertCandle(@Param("c") CandleEntity c);
}
