package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer;

import java.util.List;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class PagedSymbolsDTORespuesta {
    private List<ActiveEquityDTORespuesta> data;
    private int page;
    private int pageSize;
    private long totalElements;
    private int totalPages;
}
