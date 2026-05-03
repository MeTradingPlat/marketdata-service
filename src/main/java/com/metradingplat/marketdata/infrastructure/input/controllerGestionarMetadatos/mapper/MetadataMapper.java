package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.mapper;

import java.util.List;

import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.ReportingPolicy;

import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.ActiveEquityDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.MercadoDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.SymbolDetailsDTORespuesta;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.IGNORE)
public interface MetadataMapper {

    @Mapping(target = "id", source = "code")
    @Mapping(target = "nombre", source = "name")
    MercadoDTORespuesta deDominioARespuesta(EnumMercado mercado);

    List<MercadoDTORespuesta> deMercadosARespuestas(List<EnumMercado> mercados);

    ActiveEquityDTORespuesta deDominioARespuesta(ActiveEquity activeEquity);

    List<ActiveEquityDTORespuesta> deActiveEquitiesARespuestas(List<ActiveEquity> activeEquities);

    default SymbolDetailsDTORespuesta deActiveEquityYSymbolDetailsARespuesta(ActiveEquity activeEquity, com.metradingplat.marketdata.domain.models.FundamentalData fundamentalData) {
        return SymbolDetailsDTORespuesta.builder()
                .activeEquity(deDominioARespuesta(activeEquity))
                .fundamentalData(fundamentalData)
                .build();
    }
}
