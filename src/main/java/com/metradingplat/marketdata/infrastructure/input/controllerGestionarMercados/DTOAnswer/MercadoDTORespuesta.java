package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMercados.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class MercadoDTORespuesta {
    private String code;
    private String name;
}
