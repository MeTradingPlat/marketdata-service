package com.metradingplat.marketdata.infrastructure.input.rest;

import java.time.OffsetDateTime;
import java.util.List;
import java.util.Map;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.output.GestionarComunicacionExternalGatewayIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.DxLinkClient;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Controlador temporal para pruebas de TastyTrade.
 * ELIMINAR en producción.
 */
@RestController
@RequestMapping("/api/test")
@RequiredArgsConstructor
@Slf4j
public class TestController {

    private final GestionarComunicacionExternalGatewayIntPort tastyTradeGateway;
    private final DxLinkClient dxLinkClient;

    /**
     * Suscribirse a datos real-time de un símbolo.
     * Los datos llegarán a Kafka automáticamente.
     *
     * Ejemplo: POST http://localhost:8080/api/test/subscribe/AAPL
     */
    @PostMapping("/subscribe/{symbol}")
    public String subscribe(@PathVariable String symbol) {
        log.info("Test: subscribing to {}", symbol);
        tastyTradeGateway.subscribe(symbol);
        return "Subscribed to " + symbol + ". Check Kafka topic 'marketdata.stream' for data.";
    }

    /**
     * Desuscribirse de un símbolo.
     *
     * Ejemplo: POST http://localhost:8080/api/test/unsubscribe/AAPL
     */
    @PostMapping("/unsubscribe/{symbol}")
    public String unsubscribe(@PathVariable String symbol) {
        log.info("Test: unsubscribing from {}", symbol);
        tastyTradeGateway.unsubscribe(symbol);
        return "Unsubscribed from " + symbol;
    }

    /**
     * Obtener candles históricos.
     *
     * Ejemplo: GET http://localhost:8080/api/test/candles/AAPL?timeframe=M5&hours=2
     */
    @GetMapping("/candles/{symbol}")
    public List<Candle> getCandles(
            @PathVariable String symbol,
            @RequestParam(defaultValue = "M5") String timeframe,
            @RequestParam(defaultValue = "2") int hours) {

        log.info("Test: getting {} candles for {} (last {} hours)", timeframe, symbol, hours);

        OffsetDateTime to = OffsetDateTime.now();
        OffsetDateTime from = to.minusHours(hours);
        EnumTimeframe tf = EnumTimeframe.valueOf(timeframe);

        return tastyTradeGateway.getCandles(symbol, tf, from, to);
    }

    /**
     * Obtener el estado de la conexión DxLink.
     * Útil para monitoreo y debugging.
     *
     * Ejemplo: GET http://localhost:8080/api/test/status
     */
    @GetMapping("/status")
    public Map<String, Object> getStatus() {
        log.info("Test: getting DxLink connection status");
        return dxLinkClient.getConnectionStats();
    }

    /**
     * Forzar reconexión del cliente DxLink.
     * Útil cuando la conexión está en mal estado.
     *
     * Ejemplo: POST http://localhost:8080/api/test/reconnect
     */
    @PostMapping("/reconnect")
    public String forceReconnect() {
        log.info("Test: forcing DxLink reconnection");
        dxLinkClient.forceReconnect();
        return "Reconnection initiated. Check /status for connection state.";
    }
}
