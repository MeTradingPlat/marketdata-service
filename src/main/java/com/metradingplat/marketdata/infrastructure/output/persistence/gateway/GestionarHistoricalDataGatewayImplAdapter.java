package com.metradingplat.marketdata.infrastructure.output.persistence.gateway;

import java.time.Instant;
import java.time.OffsetDateTime;
import java.util.List;

import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import com.metradingplat.marketdata.application.output.GestionarHistoricalDataGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.CandleEntity;
import com.metradingplat.marketdata.infrastructure.output.persistence.mappers.CandleMapperPersistencia;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.SymbolEntity;
import com.metradingplat.marketdata.infrastructure.output.persistence.repositories.CandleRepository;
import com.metradingplat.marketdata.infrastructure.output.persistence.repositories.SymbolRepository;

import lombok.RequiredArgsConstructor;

@Service
@RequiredArgsConstructor
public class GestionarHistoricalDataGatewayImplAdapter implements GestionarHistoricalDataGatewayIntPort {

    private final CandleRepository objCandleRepository;
    private final SymbolRepository objSymbolRepository;
    private final CandleMapperPersistencia objMapper;

    @Override
    @Transactional(readOnly = true)
    public List<Candle> getHistoricalData(String symbol, EnumTimeframe timeframe, OffsetDateTime from,
            OffsetDateTime to) {

        Instant fromInstant = from.toInstant();
        Instant toInstant = to.toInstant();

        List<CandleEntity> entities = this.objCandleRepository.findBySymbolAndTimeframeAndRange(symbol, timeframe,
                fromInstant, toInstant);

        return this.objMapper.mappearListaEntityADominio(entities);
    }

    @Override
    @Transactional(readOnly = true)
    public long countData(String symbol, EnumTimeframe timeframe, OffsetDateTime from, OffsetDateTime to) {
        return this.objCandleRepository.countBySymbolAndTimeframeAndRange(symbol, timeframe, from.toInstant(),
                to.toInstant());
    }

    @Override
    @Transactional
    public void saveCandles(List<Candle> candles) {
        if (candles == null || candles.isEmpty())
            return;

        String ticker = candles.get(0).getSymbol();
        SymbolEntity symbolEntity = this.objSymbolRepository.findBySymbol(ticker)
                .orElseGet(() -> this.objSymbolRepository.save(SymbolEntity.builder().symbol(ticker).build()));

        for (Candle domainCandle : candles) {
            CandleEntity entity = this.objMapper.mappearDominioAEntity(domainCandle);
            entity.setSymbolObject(symbolEntity);
            this.objCandleRepository.save(entity);
        }
    }
}
