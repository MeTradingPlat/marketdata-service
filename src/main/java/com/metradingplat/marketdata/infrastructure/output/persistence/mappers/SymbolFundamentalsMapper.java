package com.metradingplat.marketdata.infrastructure.output.persistence.mappers;

import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.output.persistence.entities.SymbolFundamentalsEntity;
import org.mapstruct.Mapper;

import org.mapstruct.ReportingPolicy;

@Mapper(componentModel = "spring", unmappedTargetPolicy = ReportingPolicy.IGNORE)
public interface SymbolFundamentalsMapper {
    
    FundamentalData toDomain(SymbolFundamentalsEntity entity);
    
    SymbolFundamentalsEntity toEntity(FundamentalData domain);
}
