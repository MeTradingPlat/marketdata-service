package com.metradingplat.marketdata.infrastructure.input.health;

import java.util.Map;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.DxLinkClient;
import com.metradingplat.marketdata.infrastructure.output.external.tastytrade.TastyTradeService;

import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Controlador para monitoreo de salud y estado de la aplicación.
 * Expone endpoints para verificar el estado de las conexiones externas.
 */
@RestController
@RequestMapping("/marketdata/health")
@RequiredArgsConstructor
@Slf4j
public class HealthController {

    private final DxLinkClient dxLinkClient;
    private final TastyTradeService tastyTradeService;

    /**
     * Estado del preload masivo de fundamentales al arrancar el servicio.
     * "complete" no significa que todos los símbolos tengan datos (algunos
     * pueden no existir en TastyTrade), sino que ya se recorrió todo el
     * universo objetivo al menos una vez.
     *
     * Ejemplo: GET /marketdata/health/fundamentals
     */
    @GetMapping("/fundamentals")
    public Map<String, Object> getFundamentalsPreloadStatus() {
        return tastyTradeService.getFundamentalsPreloadStatus();
    }

    /**
     * Obtener el estado de la conexión DxLink.
     * Útil para monitoreo y debugging.
     *
     * Ejemplo: GET /marketdata/health/dxlink/status
     */
    @GetMapping("/dxlink/status")
    public Map<String, Object> getDxLinkStatus() {
        log.debug("Getting DxLink connection status");
        return dxLinkClient.getConnectionStats();
    }

    /**
     * Forzar reconexión del cliente DxLink.
     * Útil cuando la conexión está en mal estado.
     *
     * Ejemplo: POST /marketdata/health/dxlink/reconnect
     */
    @PostMapping("/dxlink/reconnect")
    public Map<String, Object> forceReconnect() {
        log.info("Forcing DxLink reconnection");
        dxLinkClient.forceReconnect();
        return Map.of(
                "message", "Reconnection initiated",
                "status", dxLinkClient.getConnectionStats());
    }
}
