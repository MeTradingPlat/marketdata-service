package com.metradingplat.marketdata.infrastructure.output.persistence.mappers;

import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.CandleEntity;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.SymbolEntity;
import java.util.ArrayList;
import java.util.List;
import javax.annotation.processing.Generated;
import org.springframework.stereotype.Component;

@Generated(
    value = "org.mapstruct.ap.MappingProcessor",
    date = "2026-01-11T18:41:38-0500",
    comments = "version: 1.5.5.Final, compiler: Eclipse JDT (IDE) 3.45.0.v20260101-2150, environment: Java 21.0.9 (Eclipse Adoptium)"
)
@Component
public class CandleMapperPersistenciaImpl implements CandleMapperPersistencia {

    @Override
    public Candle mappearEntityADominio(CandleEntity entity) {
        if ( entity == null ) {
            return null;
        }

        Candle.CandleBuilder candle = Candle.builder();

        candle.symbol( entitySymbolObjectSymbol( entity ) );
        candle.timeframe( entity.getTimeframe() );
        candle.timestamp( entity.getTimestamp() );
        candle.open( entity.getOpen() );
        candle.high( entity.getHigh() );
        candle.low( entity.getLow() );
        candle.close( entity.getClose() );
        candle.volume( entity.getVolume() );

        return candle.build();
    }

    @Override
    public List<Candle> mappearListaEntityADominio(List<CandleEntity> entities) {
        if ( entities == null ) {
            return null;
        }

        List<Candle> list = new ArrayList<Candle>( entities.size() );
        for ( CandleEntity candleEntity : entities ) {
            list.add( mappearEntityADominio( candleEntity ) );
        }

        return list;
    }

    @Override
    public CandleEntity mappearDominioAEntity(Candle candle) {
        if ( candle == null ) {
            return null;
        }

        CandleEntity.CandleEntityBuilder candleEntity = CandleEntity.builder();

        candleEntity.symbolObject( candleToSymbolEntity( candle ) );
        candleEntity.timeframe( candle.getTimeframe() );
        candleEntity.timestamp( candle.getTimestamp() );
        candleEntity.open( candle.getOpen() );
        candleEntity.high( candle.getHigh() );
        candleEntity.low( candle.getLow() );
        candleEntity.close( candle.getClose() );
        candleEntity.volume( candle.getVolume() );

        return candleEntity.build();
    }

    private String entitySymbolObjectSymbol(CandleEntity candleEntity) {
        if ( candleEntity == null ) {
            return null;
        }
        SymbolEntity symbolObject = candleEntity.getSymbolObject();
        if ( symbolObject == null ) {
            return null;
        }
        String symbol = symbolObject.getSymbol();
        if ( symbol == null ) {
            return null;
        }
        return symbol;
    }

    protected SymbolEntity candleToSymbolEntity(Candle candle) {
        if ( candle == null ) {
            return null;
        }

        SymbolEntity.SymbolEntityBuilder symbolEntity = SymbolEntity.builder();

        symbolEntity.symbol( candle.getSymbol() );

        return symbolEntity.build();
    }
}
