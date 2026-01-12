package com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure;

import lombok.Getter;
import lombok.RequiredArgsConstructor;

@Getter
@RequiredArgsConstructor
public enum CodigoError {
    ERROR_GENERICO("MD-0001", "An unexpected error occurred"),
    ENTIDAD_YA_EXISTE("MD-0002", "The entity already exists"),
    ENTIDAD_NO_ENCONTRADA("MD-0003", "The entity was not found"),
    VIOLACION_REGLA_DE_NEGOCIO("MD-0004", "Business rule violation hit"),
    ERROR_VALIDACION("MD-0005", "Validation error occurred"),
    TIPO_DE_ARGUMENTO_INVALIDO("MD-0006", "Invalid argument type provided");

    private final String codigo;
    private final String mensajeDefault;
}
