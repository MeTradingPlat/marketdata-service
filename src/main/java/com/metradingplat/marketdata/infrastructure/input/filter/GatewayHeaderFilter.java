package com.metradingplat.marketdata.infrastructure.input.filter;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;

import org.springframework.lang.NonNull;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;

/**
 * Filtro de seguridad que verifica que las peticiones HTTP pasen por el API Gateway.
 * Solo permite peticiones que contengan el header X-Gateway-Passed=true.
 */
@Component
public class GatewayHeaderFilter extends OncePerRequestFilter {

    private static final String GATEWAY_HEADER = "X-Gateway-Passed";
    private static final String GATEWAY_HEADER_VALUE = "true";

    @Override
    protected void doFilterInternal(@NonNull HttpServletRequest request, @NonNull HttpServletResponse response,
            @NonNull FilterChain filterChain)
            throws ServletException, IOException {

        String header = request.getHeader(GATEWAY_HEADER);

        if (header == null || !header.equals(GATEWAY_HEADER_VALUE)) {
            response.sendError(HttpServletResponse.SC_FORBIDDEN, "Access denied: Request must pass through API Gateway");
            return;
        }

        filterChain.doFilter(request, response);
    }
}
