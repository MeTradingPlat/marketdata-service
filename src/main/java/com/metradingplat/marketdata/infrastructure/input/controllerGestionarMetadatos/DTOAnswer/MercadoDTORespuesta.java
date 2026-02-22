package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class MercadoDTORespuesta {
    private String id;
    private String nombre;
}
