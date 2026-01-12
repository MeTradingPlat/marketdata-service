package com.metradingplat.marketdata.infrastructure.output.exceptionsController.ownExceptions;

import com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure.CodigoError;

import lombok.Getter;

@Getter
public class BaseException extends RuntimeException {
    private final CodigoError codigoError;
    private final Object[] args;

    public BaseException(CodigoError codigoError, String message, Object... args) {
        super(message);
        this.codigoError = codigoError;
        this.args = args;
    }
}
