package com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.mapper;

import org.mapstruct.Mapper;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarFundamentals.DTOAnswer.FundamentalDataDTORespuesta;

@Mapper(componentModel = "spring")
public interface FundamentalsMapper {
    FundamentalDataDTORespuesta toDTORespuesta(FundamentalData domainModel);
}
