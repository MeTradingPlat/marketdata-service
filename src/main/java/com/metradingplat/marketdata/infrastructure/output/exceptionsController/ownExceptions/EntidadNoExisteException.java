package com.metradingplat.marketdata.infrastructure.output.exceptionsController.ownExceptions;

import com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure.CodigoError;

public class EntidadNoExisteException extends BaseException {
    public EntidadNoExisteException(String message, Object... args) {
        super(CodigoError.ENTIDAD_NO_ENCONTRADA, message, args);
    }
}
