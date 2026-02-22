package com.metradingplat.marketdata.infrastructure.output.persistence.mappers;

import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.persistence.entities.CandleEntity;
import org.mapstruct.Mapper;
import org.mapstruct.Mapping;

import java.util.List;

@Mapper(componentModel = "spring")
public interface CandlePersistenceMapper {
    Candle toDomain(CandleEntity entity);

    @Mapping(target = "id", ignore = true)
    CandleEntity toEntity(Candle domain);

    List<Candle> toDomainList(List<CandleEntity> entities);

    List<CandleEntity> toEntityList(List<Candle> domains);
}
