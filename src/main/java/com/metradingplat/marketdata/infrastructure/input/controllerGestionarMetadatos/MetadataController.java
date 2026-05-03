package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos;

import java.util.Arrays;
import java.util.List;

import org.springframework.context.MessageSource;
import org.springframework.context.i18n.LocaleContextHolder;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarFundamentalsCUIntPort;
import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumMercado;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.domain.models.FundamentalData;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.ActiveEquityDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.MercadoDTORespuesta;
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

        @GetMapping("/timeframes")
        public List<TimeframeDTORespuesta> getTimeframes() {
                return Arrays.stream(EnumTimeframe.values())
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
                List<String> dynamicMarkets = this.objGestionarMercadosCUInt.obtenerMercadosDisponibles();
                
                return dynamicMarkets.stream()
                                .map(mic -> {
                                        // Attempt to find a matching EnumMercado by its MIC codes
                                        String name = Arrays.stream(EnumMercado.values())
                                                        .filter(m -> m.getMicCodes().contains(mic))
                                                        .map(EnumMercado::getName)
                                                        .findFirst()
                                                        .orElse(mic); // Fallback to MIC code if unknown

                                        return MercadoDTORespuesta.builder()
                                                        .id(mic.toLowerCase())
                                                        .nombre(name)
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
