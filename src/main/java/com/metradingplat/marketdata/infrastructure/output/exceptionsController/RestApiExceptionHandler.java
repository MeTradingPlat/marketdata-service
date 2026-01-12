package com.metradingplat.marketdata.infrastructure.output.exceptionsController;

import java.util.stream.Collectors;

import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.http.converter.HttpMessageNotReadableException;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;

import com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure.CodigoError;
import com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure.Error;
import com.metradingplat.marketdata.infrastructure.output.exceptionsController.exceptionStructure.ErrorUtils;
import com.metradingplat.marketdata.infrastructure.output.exceptionsController.ownExceptions.EntidadNoExisteException;
import com.metradingplat.marketdata.infrastructure.output.exceptionsController.ownExceptions.ReglaNegocioException;

import jakarta.servlet.http.HttpServletRequest;
import lombok.extern.slf4j.Slf4j;

@RestControllerAdvice
@Slf4j
public class RestApiExceptionHandler {

    @ExceptionHandler(Exception.class)
    public ResponseEntity<Error> handleGenericException(HttpServletRequest req, Exception ex) {
        log.error("Generic error at {}: {}", req.getRequestURL(), ex.getMessage(), ex);
        return createErrorResponse(CodigoError.ERROR_GENERICO, HttpStatus.INTERNAL_SERVER_ERROR, req, ex.getMessage());
    }

    @ExceptionHandler(EntidadNoExisteException.class)
    public ResponseEntity<Error> handleEntidadNoExiste(HttpServletRequest req, EntidadNoExisteException ex) {
        return createErrorResponse(ex.getCodigoError(), HttpStatus.NOT_FOUND, req, ex.getMessage());
    }

    @ExceptionHandler(ReglaNegocioException.class)
    public ResponseEntity<Error> handleReglaNegocio(HttpServletRequest req, ReglaNegocioException ex) {
        return createErrorResponse(ex.getCodigoError(), HttpStatus.BAD_REQUEST, req, ex.getMessage());
    }

    @ExceptionHandler(MethodArgumentNotValidException.class)
    public ResponseEntity<Error> handleValidation(HttpServletRequest req, MethodArgumentNotValidException ex) {
        String mensaje = ex.getBindingResult().getFieldErrors().stream()
                .map(e -> e.getField() + ": " + e.getDefaultMessage())
                .collect(Collectors.joining("; "));
        return createErrorResponse(CodigoError.ERROR_VALIDACION, HttpStatus.BAD_REQUEST, req, mensaje);
    }

    @ExceptionHandler(MethodArgumentTypeMismatchException.class)
    public ResponseEntity<Error> handleTypeMismatch(HttpServletRequest req, MethodArgumentTypeMismatchException ex) {
        String mensaje = String.format("Parameter '%s' should be of type %s", ex.getName(),
                ex.getRequiredType().getSimpleName());
        return createErrorResponse(CodigoError.TIPO_DE_ARGUMENTO_INVALIDO, HttpStatus.BAD_REQUEST, req, mensaje);
    }

    @ExceptionHandler(HttpMessageNotReadableException.class)
    public ResponseEntity<Error> handleJsonError(HttpServletRequest req, HttpMessageNotReadableException ex) {
        return createErrorResponse(CodigoError.VIOLACION_REGLA_DE_NEGOCIO, HttpStatus.BAD_REQUEST, req,
                "Malformed JSON request");
    }

    private ResponseEntity<Error> createErrorResponse(CodigoError codigoError, HttpStatus status,
            HttpServletRequest req, String message) {
        Error error = ErrorUtils.crearError(
                codigoError.getCodigo(),
                message != null ? message : codigoError.getMensajeDefault(),
                status.value(),
                req.getRequestURL().toString(),
                req.getMethod());
        return ResponseEntity.status(status).body(error);
    }
}
