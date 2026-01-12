package com.metradingplat.marketdata.infrastructure.output.persistence.mappers;

import java.util.List;

import org.mapstruct.Mapper;
import org.mapstruct.Mapping;

import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.persistence.entitys.CandleEntity;

@Mapper(componentModel = "spring")
public interface CandleMapperPersistencia {

    @Mapping(source = "symbolObject.symbol", target = "symbol")
    Candle mappearEntityADominio(CandleEntity entity);

    List<Candle> mappearListaEntityADominio(List<CandleEntity> entities);

    @Mapping(source = "symbol", target = "symbolObject.symbol")
    @Mapping(target = "id", ignore = true)
    CandleEntity mappearDominioAEntity(Candle candle);
}
