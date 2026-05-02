package com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.mapper;

import org.mapstruct.Mapper;
import com.metradingplat.marketdata.domain.models.OptionChain;
import com.metradingplat.marketdata.domain.models.OptionContract;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.DTOAnswer.OptionChainDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarOptions.DTOAnswer.OptionContractDTORespuesta;

@Mapper(componentModel = "spring")
public interface OptionsMapper {
    OptionChainDTORespuesta toDTORespuesta(OptionChain domainModel);
    OptionContractDTORespuesta toDTORespuesta(OptionContract domainModel);
    java.util.List<OptionContractDTORespuesta> toDTOList(java.util.List<OptionContract> list);
    java.util.Map<String, java.util.List<OptionContractDTORespuesta>> mapExpirations(java.util.Map<String, java.util.List<OptionContract>> value);
}
