package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos;

import java.util.List;
import java.util.Map;

import org.springframework.context.MessageSource;
import org.springframework.context.i18n.LocaleContextHolder;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarFundamentalsCUIntPort;
import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.domain.models.PagedActiveEquities;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.ActiveEquityDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.MercadoDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.PagedSymbolsDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.SymbolDetailsDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.TimeframeDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.mapper.MetadataMapper;

import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/marketdata")
@RequiredArgsConstructor
public class MetadataController {

        private final MessageSource messageSource;
        private final GestionarMercadosCUIntPort objGestionarMercadosCUInt;
        private final GestionarFundamentalsCUIntPort objGestionarFundamentalsCUInt;
        private final MetadataMapper objMapper;

        // Dynamic mapping for MIC codes to friendly names as TastyTrade doesn't provide them in a master list
        private static final Map<String, String> MARKET_NAME_MAP = Map.of(
            "XNAS", "NASDAQ",
            "XNYS", "NYSE",
            "XASE", "AMEX",
            "ARCX", "NYSE ARCA (ETFs)",
            "BATS", "CBOE BZX (ETFs)",
            "OTC", "OTC Markets",
            "CME", "Chicago Mercantile Exchange",
            "CFE", "CBOE Futures Exchange"
        );

        @GetMapping("/timeframes")
        public List<TimeframeDTORespuesta> getTimeframes() {
                return java.util.Arrays.stream(EnumTimeframe.values())
                                .map(tf -> TimeframeDTORespuesta.builder()
                                                .id(tf.name())
                                                .codigo(tf.getLabel())
                                                .nombre(messageSource.getMessage("timeframe." + tf.name().toLowerCase(),
                                                                null,
                                                                tf.name(),
                                                                LocaleContextHolder.getLocale()))
                                                .build())
                                .toList();
        }

        @GetMapping("/markets")
        public List<MercadoDTORespuesta> getMarkets() {
                // Discovery: Get unique markets currently active in TastyTrade
                List<String> discoveredMics = this.objGestionarMercadosCUInt.obtenerMercadosDisponibles();
                
                return discoveredMics.stream()
                                .map(mic -> {
                                        String upperMic = mic.toUpperCase();
                                        String friendlyName = MARKET_NAME_MAP.getOrDefault(upperMic, upperMic);
                                        
                                        return MercadoDTORespuesta.builder()
                                                        .id(mic.toLowerCase())
                                                        .nombre(friendlyName)
                                                        .build();
                                })
                                .toList();
        }

        @GetMapping("/symbols")
        public ResponseEntity<List<ActiveEquityDTORespuesta>> obtenerSimbolos(
                        @RequestParam(value = "markets", required = false) List<String> markets) {
                List<ActiveEquity> activeEquities = this.objGestionarMercadosCUInt.obtenerSimbolosPorMercados(markets);
                List<ActiveEquityDTORespuesta> respuesta = this.objMapper.deActiveEquitiesARespuestas(activeEquities);
                return ResponseEntity.ok(respuesta);
        }

        @GetMapping("/symbols/search")
        public ResponseEntity<PagedSymbolsDTORespuesta> buscarSimbolos(
                        @RequestParam(value = "q", required = false) String q,
                        @RequestParam(value = "markets", required = false) List<String> markets,
                        @RequestParam(value = "page", defaultValue = "0") int page,
                        @RequestParam(value = "size", defaultValue = "50") int size) {
                PagedActiveEquities resultado = this.objGestionarMercadosCUInt.buscarSimbolos(q, markets, page, size);
                int totalPages = size > 0 ? (int) Math.ceil(resultado.totalElements() / (double) size) : 0;
                return ResponseEntity.ok(PagedSymbolsDTORespuesta.builder()
                                .data(this.objMapper.deActiveEquitiesARespuestas(resultado.items()))
                                .page(page)
                                .pageSize(size)
                                .totalElements(resultado.totalElements())
                                .totalPages(totalPages)
                                .build());
        }

        @GetMapping("/symbols/{symbol}/details")
        public ResponseEntity<SymbolDetailsDTORespuesta> obtenerDetallesSimbolo(
                        @org.springframework.web.bind.annotation.PathVariable("symbol") String symbol) {
                // Get ActiveEquity info from cache/service
                List<ActiveEquity> activeEquities = this.objGestionarMercadosCUInt.obtenerSimbolosPorMercados(null);
                ActiveEquity activeEquity = activeEquities.stream()
                                .filter(eq -> eq.getSymbol().equalsIgnoreCase(symbol))
                                .findFirst()
                                .orElse(ActiveEquity.builder().symbol(symbol).description("").listedMarket("").build());

                // Get Fundamentals
                FundamentalData fundamentals = this.objGestionarFundamentalsCUInt.obtenerFundamentals(symbol);

                SymbolDetailsDTORespuesta respuesta = this.objMapper.deActiveEquityYSymbolDetailsARespuesta(activeEquity,
                                fundamentals);
                return ResponseEntity.ok(respuesta);
        }
}
