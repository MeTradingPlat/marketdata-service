package com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure;

public final class ErrorUtils {

    private ErrorUtils() {
    }

    public static Error crearError(final String codigoError,
            final String mensaje,
            final Integer codigoHttp,
            final String url,
            final String metodo) {
        return Error.builder()
                .codigoError(codigoError)
                .mensaje(mensaje)
                .codigoHttp(codigoHttp)
                .url(url)
                .metodo(metodo)
                .build();
    }
}
