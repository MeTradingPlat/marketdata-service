package com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.controller;

import java.time.Instant;
import java.time.OffsetDateTime;
import java.util.List;

import java.util.HashMap;
import java.util.Map;

import org.springframework.format.annotation.DateTimeFormat;
import org.springframework.http.ResponseEntity;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import com.metradingplat.marketdata.application.input.GestionarHistoricalDataCUIntPort;
import com.metradingplat.marketdata.domain.enums.EnumTimeframe;
import com.metradingplat.marketdata.domain.models.Candle;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.BatchCandlesDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.BatchSingleCandleDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOAnswer.CandleDTORespuesta;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.DTOPetition.BatchCandlesDTOPeticion;
import com.metradingplat.marketdata.infrastructure.input.controllerGestionarHistoricalData.mapper.HistoricalDataMapper;

import jakarta.validation.Valid;
import jakarta.validation.constraints.NotNull;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@RestController
@RequestMapping("/api/marketdata/historical")
@RequiredArgsConstructor
@Validated
@Slf4j
public class HistoricalDataRestController {

    private final GestionarHistoricalDataCUIntPort objGestionarHistoricalDataCUInt;
    private final HistoricalDataMapper objMapper;

    @GetMapping("/{symbol}")
    public ResponseEntity<List<CandleDTORespuesta>> getCandles(
            @PathVariable("symbol") @NotNull String symbol,
            @RequestParam("timeframe") @NotNull EnumTimeframe timeframe,
            @RequestParam(value = "endDate", required = false) @DateTimeFormat(iso = DateTimeFormat.ISO.DATE_TIME) OffsetDateTime endDate,
            @RequestParam(value = "bars", required = false) Integer bars) {

        log.info("GET /historical/{} timeframe={} endDate={} bars={}", symbol, timeframe, endDate, bars);
        List<Candle> candles = this.objGestionarHistoricalDataCUInt.getCandles(symbol, timeframe, endDate, bars);
        log.info("GET /historical/{} -> {} candles", symbol, candles.size());
        return ResponseEntity.ok(this.objMapper.deDominioARespuestas(candles));
    }

    @GetMapping("/{symbol}/current")
    public ResponseEntity<CandleDTORespuesta> getCurrentCandle(
            @PathVariable("symbol") @NotNull String symbol,
            @RequestParam("timeframe") @NotNull EnumTimeframe timeframe) {

        log.info("GET /historical/{}/current timeframe={}", symbol, timeframe);
        Candle candle = this.objGestionarHistoricalDataCUInt.getCurrentCandle(symbol, timeframe);

        if (candle == null) {
            log.info("GET /historical/{}/current -> sin datos", symbol);
            return ResponseEntity.noContent().build();
        }

        log.info("GET /historical/{}/current -> candle at {}", symbol, candle.getTimestamp());
        return ResponseEntity.ok(this.objMapper.deDominioARespuesta(candle));
    }

    @GetMapping("/{symbol}/last")
    public ResponseEntity<CandleDTORespuesta> getLastCandle(
            @PathVariable("symbol") @NotNull String symbol,
            @RequestParam("timeframe") @NotNull EnumTimeframe timeframe) {

        log.info("GET /historical/{}/last timeframe={}", symbol, timeframe);
        Candle candle = this.objGestionarHistoricalDataCUInt.getLastCandle(symbol, timeframe);

        if (candle == null) {
            log.info("GET /historical/{}/last -> sin datos", symbol);
            return ResponseEntity.noContent().build();
        }

        log.info("GET /historical/{}/last -> candle at {}", symbol, candle.getTimestamp());
        return ResponseEntity.ok(this.objMapper.deDominioARespuesta(candle));
    }

    @PostMapping("/batch")
    public ResponseEntity<BatchCandlesDTORespuesta> getCandlesBatch(
            @RequestBody @Valid BatchCandlesDTOPeticion peticion) {

        int barsReq = peticion.getBars() != null ? peticion.getBars() : 100; // Limite default mas bajo

        log.info("POST /historical/batch symbols={} timeframe={} bars={}",
                peticion.getSymbols().size(), peticion.getTimeframe(), barsReq);

        Map<String, List<Candle>> candlesDominio = this.objGestionarHistoricalDataCUInt.getCandlesBatch(
                peticion.getSymbols(),
                peticion.getTimeframe(),
                barsReq);

        // Convertir dominio a DTO
        Map<String, List<CandleDTORespuesta>> candlesDTO = new HashMap<>();
        for (Map.Entry<String, List<Candle>> entry : candlesDominio.entrySet()) {
            candlesDTO.put(entry.getKey(), this.objMapper.deDominioARespuestas(entry.getValue()));
        }

        BatchCandlesDTORespuesta respuesta = BatchCandlesDTORespuesta.builder()
                .candlesPorSimbolo(candlesDTO)
                .serverTimestamp(Instant.now())
                .build();

        log.info("POST /historical/batch -> {} simbolos, {} candles totales",
                candlesDTO.size(), candlesDTO.values().stream().mapToInt(List::size).sum());

        return ResponseEntity.ok(respuesta);
    }

    @PostMapping("/batch/last")
    public ResponseEntity<BatchSingleCandleDTORespuesta> getLastCandlesBatch(
            @RequestBody @Valid BatchCandlesDTOPeticion peticion) {

        log.info("POST /historical/batch/last symbols={} timeframe={}",
                peticion.getSymbols().size(), peticion.getTimeframe());

        Map<String, Candle> candlesDominio = this.objGestionarHistoricalDataCUInt.getLastCandleBatch(
                peticion.getSymbols(),
                peticion.getTimeframe());

        // Convertir dominio a DTO
        Map<String, CandleDTORespuesta> candlesDTO = new HashMap<>();
        for (Map.Entry<String, Candle> entry : candlesDominio.entrySet()) {
            candlesDTO.put(entry.getKey(), this.objMapper.deDominioARespuesta(entry.getValue()));
        }

        BatchSingleCandleDTORespuesta respuesta = BatchSingleCandleDTORespuesta.builder()
                .candlePorSimbolo(candlesDTO)
                .serverTimestamp(Instant.now())
                .build();

        log.info("POST /historical/batch/last -> {} simbolos con datos", candlesDTO.size());

        return ResponseEntity.ok(respuesta);
    }

    @PostMapping("/batch/current")
    public ResponseEntity<BatchSingleCandleDTORespuesta> getCurrentCandlesBatch(
            @RequestBody @Valid BatchCandlesDTOPeticion peticion) {

        log.info("POST /historical/batch/current symbols={} timeframe={}",
                peticion.getSymbols().size(), peticion.getTimeframe());

        Map<String, Candle> candlesDominio = this.objGestionarHistoricalDataCUInt.getCurrentCandleBatch(
                peticion.getSymbols(),
                peticion.getTimeframe());

        // Convertir dominio a DTO
        Map<String, CandleDTORespuesta> candlesDTO = new HashMap<>();
        for (Map.Entry<String, Candle> entry : candlesDominio.entrySet()) {
            candlesDTO.put(entry.getKey(), this.objMapper.deDominioARespuesta(entry.getValue()));
        }

        BatchSingleCandleDTORespuesta respuesta = BatchSingleCandleDTORespuesta.builder()
                .candlePorSimbolo(candlesDTO)
                .serverTimestamp(Instant.now())
                .build();

        log.info("POST /historical/batch/current -> {} simbolos con datos", candlesDTO.size());

        return ResponseEntity.ok(respuesta);
    }
}
