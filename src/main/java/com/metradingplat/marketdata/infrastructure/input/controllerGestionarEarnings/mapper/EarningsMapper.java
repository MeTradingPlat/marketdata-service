package com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.mapper;

import org.mapstruct.Mapper;
import org.mapstruct.ReportingPolicy;

import com.metradingplat.marketdata.domain.models.EarningsReport;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarEarnings.DTOAnswer.EarningsReportDTORespuesta;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.IGNORE)
public interface EarningsMapper {
    EarningsReportDTORespuesta deDominioARespuesta(EarningsReport earningsReport);
}
