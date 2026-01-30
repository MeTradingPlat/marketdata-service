package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.controller;

import java.time.OffsetDateTime;
import java.util.List;

import org.springframework.format.annotation.DateTimeFormat;
import org.springframework.http.ResponseEntity;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.CandleDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.mapper.HistoricalDataMapper;

import jakarta.validation.constraints.NotNull;
import lombok.RequiredArgsConstructor;

@RestController
@RequestMapping("/api/marketdata/historical")
@RequiredArgsConstructor
@Validated
public class HistoricalDataRestController {

    private final GestionarHistoricalDataCUIntPort objGestionarHistoricalDataCUInt;
    private final HistoricalDataMapper objMapper;

    @GetMapping("/{symbol}")
    public ResponseEntity<List<CandleDTORespuesta>> getHistoricalData(
            @PathVariable("symbol") @NotNull String symbol,
            @RequestParam("timeframe") @NotNull EnumTimeframe timeframe,
            @RequestParam("from") @NotNull @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) OffsetDateTime from,
            @RequestParam("to") @NotNull @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) OffsetDateTime to) {

        List<Candle> candles = this.objGestionarHistoricalDataCUInt.getHistoricalMarketData(symbol, timeframe, from,
                to);
        List<CandleDTORespuesta> respuesta = this.objMapper.deDominioARespuestas(candles);

        return ResponseEntity.ok(respuesta);
    }

    @GetMapping("/{symbol}/last")
    public ResponseEntity<CandleDTORespuesta> getLastCandle(
            @PathVariable("symbol") @NotNull String symbol,
            @RequestParam("timeframe") @NotNull EnumTimeframe timeframe) {

        Candle candle = this.objGestionarHistoricalDataCUInt.getLastCandle(symbol, timeframe);

        if (candle == null) {
            return ResponseEntity.noContent().build();
        }

        return ResponseEntity.ok(this.objMapper.deDominioARespuesta(candle));
    }
}
