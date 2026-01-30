package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.mapper;

import java.util.List;

import org.mapstruct.Mapper;
import org.mapstruct.ReportingPolicy;

import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.DTOAnswer.ActiveEquityDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.DTOAnswer.MercadoDTORespuesta;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.IGNORE)
public interface MercadosMapper {

    MercadoDTORespuesta deDominioARespuesta(EnumMercado mercado);

    List<MercadoDTORespuesta> deMercadosARespuestas(List<EnumMercado> mercados);

    ActiveEquityDTORespuesta deDominioARespuesta(ActiveEquity activeEquity);

    List<ActiveEquityDTORespuesta> deActiveEquitiesARespuestas(List<ActiveEquity> activeEquities);
}
