package com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.mapper;

import org.mapstruct.Mapper;
import org.mapstruct.ReportingPolicy;

import com.metradingplat.marketdata.domain.models.Quote;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarQuote.DTOAnswer.QuoteDTORespuesta;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.IGNORE)
public interface QuoteMapper {
    QuoteDTORespuesta deDominioARespuesta(Quote quote);
}
