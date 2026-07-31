package com.metradingplat.marketdata.domain.models;

import java.util.List;

public record PagedActiveEquities(List<ActiveEquity> items, long totalElements) {
}
