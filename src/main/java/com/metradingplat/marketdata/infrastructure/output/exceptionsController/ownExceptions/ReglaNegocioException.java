package com.metradingplat.marketdata.infrastructure.output.exceptionsController.ownExceptions;

import com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure.CodigoError;

public class ReglaNegocioException extends BaseException {
    public ReglaNegocioException(String message, Object... args) {
        super(CodigoError.VIOLACION_REGLA_DE_NEGOCIO, message, args);
    }
}
