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
                                                .id("nyse")
                                                .nombre(messageSource.getMessage("market.nyse", null, "NYSE",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("nasdaq")
                                                .nombre(messageSource.getMessage("market.nasdaq", null, "NASDAQ",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("amex")
                                                .nombre(messageSource.getMessage("market.amex", null, "AMEX",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("arca")
                                                .nombre(messageSource.getMessage("market.arca", null, "ARCA",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("bats")
                                                .nombre(messageSource.getMessage("market.bats", null, "BATS",
                                                                LocaleContextHolder.getLocale()))
                                                .build(),
                                MercadoDTORespuesta.builder()
                                                .id("otc")
                                                .nombre(messageSource.getMessage("market.otc", null, "OTC",
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
