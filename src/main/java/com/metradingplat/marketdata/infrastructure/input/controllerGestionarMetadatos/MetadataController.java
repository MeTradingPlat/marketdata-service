package com.metradingplat.marketdata.infrastructure.input.controllerGestionarMetadatos;

import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import lombok.RequiredArgsConstructor;
import org.springframework.context.MessageSource;
import org.springframework.context.i18n.LocaleContextHolder;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Arrays;
import java.util.List;
import java.util.Map;

@RestController
@RequestMapping("/api/marketdata")
@RequiredArgsConstructor
public class MetadataController {

    private final MessageSource messageSource;

    @GetMapping("/timeframes")
    public List<Map<String, String>> getTimeframes() {
        return Arrays.stream(EnumTimeframe.values())
                .map(tf -> Map.of(
                        "id", tf.name(),
                        "codigo", tf.getLabel(),
                        "nombre",
                        messageSource.getMessage("timeframe." + tf.name().toLowerCase(), null, tf.name(),
                                LocaleContextHolder.getLocale())))
                .toList();
    }

    @GetMapping("/markets")
    public List<Map<String, String>> getMarkets() {
        return List.of(
                Map.of("id", "us_equities", "nombre",
                        messageSource.getMessage("market.us_equities", null, "US Equities",
                                LocaleContextHolder.getLocale())),
                Map.of("id", "crypto", "nombre",
                        messageSource.getMessage("market.crypto", null, "Crypto", LocaleContextHolder.getLocale())),
                Map.of("id", "forex", "nombre",
                        messageSource.getMessage("market.forex", null, "Forex", LocaleContextHolder.getLocale())));
    }
}
