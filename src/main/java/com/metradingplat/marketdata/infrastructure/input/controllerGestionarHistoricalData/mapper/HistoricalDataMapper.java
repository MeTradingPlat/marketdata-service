package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.mapper;

import java.util.List;
import org.mapstruct.Mapper;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.CandleDTORespuesta;

@Mapper(componentModel = "spring")
public interface HistoricalDataMapper {
    CandleDTORespuesta deDominioARespuesta(Candle candle);

    List<CandleDTORespuesta> deDominioARespuestas(List<Candle> candles);
}
