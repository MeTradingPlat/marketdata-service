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

import com.metradingplat.marketdata.application.input.GestionarMercadosCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.ActiveEquity;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.ActiveEquityDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.MercadoDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.DTOAnswer.TimeframeDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos.mapper.MetadataMapper;

import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/marketdata")
@RequiredArgsConstructor
public class MetadataController {

        private final MessageSource messageSource;
        private final GestionarMercadosCUIntPort objGestionarMercadosCUInt;
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
                return List.of(
                                MercadoDTORespuesta.builder()
                                                .id("us_equities")
                                                .nombre(messageSource.getMessage("market.us_equities", null,
                                                                "US Equities",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("crypto")
                                                .nombre(messageSource.getMessage("market.crypto", null, "Crypto",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("forex")
                                                .nombre(messageSource.getMessage("market.forex", null, "Forex",
                                                                LocaleContextHolder.getLocale()))
                                                .build());
        }

        @GetMapping("/symbols")
        public ResponseEntity<List<ActiveEquityDTORespuesta>> obtenerSimbolos(
                        @RequestParam("markets") List<String> markets) {
                List<ActiveEquity> activeEquities = this.objGestionarMercadosCUInt.obtenerSimbolosPorMercados(markets);
                List<ActiveEquityDTORespuesta> respuesta = this.objMapper.deActiveEquitiesARespuestas(activeEquities);
                return ResponseEntity.ok(respuesta);
        }
}
