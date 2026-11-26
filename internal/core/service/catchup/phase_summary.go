package catchup

import "sort"

const missingSampleLimit = 20

type phaseSummary struct {
	requested      int
	saved          int
	empty          []string
	cut            int
	upToDate       int
	skippedLive    int
	errored        int
	noWatermark    int
	partialBatches int
	accounted      map[string]struct{}
}

func newPhaseSummary(uncovered, covered []job) *phaseSummary {
	total := len(uncovered) + len(covered)
	return &phaseSummary{
		requested: total,
		accounted: make(map[string]struct{}, total),
	}
}

func (s *phaseSummary) merge(o batchOutcome) {
	s.saved += o.saved
	s.empty = append(s.empty, o.empty...)
	s.cut += o.cut
	s.upToDate += o.upToDate
	s.skippedLive += o.skippedLive
	s.errored += o.errored
	s.noWatermark += o.noWatermark
	if o.partial {
		s.partialBatches++
	}
	for _, sym := range o.accounted {
		s.accounted[sym] = struct{}{}
	}
}

func (s *phaseSummary) missing(uncovered, covered []job) (int, []string) {
	accountedCount := s.saved + len(s.empty) + s.cut + s.upToDate + s.skippedLive + s.errored
	missingCount := max(0, s.requested-accountedCount)
	if missingCount == 0 {
		return 0, nil
	}
	sample := make([]string, 0, min(missingCount, missingSampleLimit))
	for _, jobs := range [][]job{uncovered, covered} {
		for _, j := range jobs {
			if _, ok := s.accounted[j.symbol]; ok {
				continue
			}
			sample = append(sample, j.symbol)
			if len(sample) >= missingSampleLimit {
				sort.Strings(sample)
				return missingCount, sample
			}
		}
	}
	sort.Strings(sample)
	return missingCount, sample
}
