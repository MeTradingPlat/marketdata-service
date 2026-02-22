package com.metradingplat.marketdata.infrastructure.output.persistence.gateway;

import com.metradingplat.marketdata.application.output.GestionarCandleRepositoryIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.persistence.mappers.CandlePersistenceMapper;
import com.metradingplat.marketdata.infrastructure.output.persistence.repositories.CandleRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.data.domain.PageRequest;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

@Component
@RequiredArgsConstructor
public class CandleRepositoryGatewayImplAdapter implements GestionarCandleRepositoryIntPort {

    private final CandleRepository candleRepository;
    private final CandlePersistenceMapper candleMapper;

    @Override
    public void upsert(Candle candle) {
        candleRepository.upsertCandle(candleMapper.toEntity(candle));
    }

    @Override
    public Candle findMostRecent(String symbol, EnumTimeframe timeframe) {
        var entity = candleRepository.findTopBySymbolAndTimeframeOrderByTimestampDesc(symbol, timeframe);
        return entity != null ? candleMapper.toDomain(entity) : null;
    }

    @Override
    public List<Candle> findLatest(String symbol, EnumTimeframe timeframe, int limit) {
        var entities = candleRepository.findLatestCandles(symbol, timeframe, PageRequest.of(0, limit));
        List<Candle> result = new ArrayList<>(candleMapper.toDomainList(entities));
        // The JPA query returns DESC order (latest first), reverse to ascending for
        // consumers
        result.sort((a, b) -> a.getTimestamp().compareTo(b.getTimestamp()));
        return result;
    }
}
