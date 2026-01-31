package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.mapper;

import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.CandleDTORespuesta;
import java.util.ArrayList;
import java.util.List;
import javax.annotation.processing.Generated;
import org.springframework.stereotype.Component;

@Generated(
    value = "org.mapstruct.ap.MappingProcessor",
    date = "2026-01-31T13:59:37-0500",
    comments = "version: 1.5.5.Final, compiler: Eclipse JDT (IDE) 3.45.0.v20260128-0750, environment: Java 21.0.9 (Eclipse Adoptium)"
)
@Component
public class HistoricalDataMapperImpl implements HistoricalDataMapper {

    @Override
    public CandleDTORespuesta deDominioARespuesta(Candle candle) {
        if ( candle == null ) {
            return null;
        }

        CandleDTORespuesta.CandleDTORespuestaBuilder candleDTORespuesta = CandleDTORespuesta.builder();

        candleDTORespuesta.close( candle.getClose() );
        candleDTORespuesta.high( candle.getHigh() );
        candleDTORespuesta.low( candle.getLow() );
        candleDTORespuesta.open( candle.getOpen() );
        candleDTORespuesta.symbol( candle.getSymbol() );
        candleDTORespuesta.timestamp( candle.getTimestamp() );
        candleDTORespuesta.volume( candle.getVolume() );

        return candleDTORespuesta.build();
    }

    @Override
    public List<CandleDTORespuesta> deDominioARespuestas(List<Candle> candles) {
        if ( candles == null ) {
            return null;
        }

        List<CandleDTORespuesta> list = new ArrayList<CandleDTORespuesta>( candles.size() );
        for ( Candle candle : candles ) {
            list.add( deDominioARespuesta( candle ) );
        }

        return list;
    }
}
