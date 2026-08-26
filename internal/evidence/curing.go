package evidence

import (
	"fmt"
	"sort"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/fixed"
)

// CuringInterval is a contiguous temperature/humidity reading interval with a
// rate factor expressed as a scaled fixed-point ratio. Equivalent curing time
// accumulates rate * duration for every interval.
type CuringInterval struct {
	Start, End       domain.LogicalTime
	RateNum, RateDen int64
}

// SortIntervals orders intervals by start time deterministically.
func SortIntervals(intervals []CuringInterval) {
	sort.SliceStable(intervals, func(i, j int) bool {
		return intervals[i].Start < intervals[j].Start
	})
}

// CheckContinuity verifies the timeline has no gaps, no overlaps and no
// negative or inverted durations. A gap blocks handover and triggers a retest.
func CheckContinuity(intervals []CuringInterval) error {
	sorted := make([]CuringInterval, len(intervals))
	copy(sorted, intervals)
	SortIntervals(sorted)
	for i, iv := range sorted {
		if iv.End < iv.Start {
			return fmt.Errorf("evidence: inverted curing interval")
		}
		if i == 0 {
			continue
		}
		prev := sorted[i-1]
		if iv.Start != prev.End {
			return fmt.Errorf("evidence: curing gap between %d and %d", prev.End, iv.Start)
		}
	}
	return nil
}

// IntegrateCuring computes the total equivalent curing time in logical seconds
// by integer fixed-point integration: sum over intervals of
// (End-Start)*RateNum/RateDen. Any overflow, negative rate or zero denominator
// aborts the whole command without writing intermediate results.
func IntegrateCuring(intervals []CuringInterval) (int64, error) {
	var total int64
	for _, iv := range intervals {
		if iv.End < iv.Start {
			return 0, fmt.Errorf("evidence: inverted curing interval")
		}
		if iv.RateNum < 0 || iv.RateDen <= 0 {
			return 0, fmt.Errorf("evidence: illegal curing rate")
		}
		duration := int64(iv.End - iv.Start)
		eq, err := fixed.MulDivChecked(duration, iv.RateNum, iv.RateDen)
		if err != nil {
			return 0, fmt.Errorf("evidence: curing integration %w", err)
		}
		total, err = fixed.AddChecked(total, eq)
		if err != nil {
			return 0, fmt.Errorf("evidence: curing integration %w", err)
		}
	}
	return total, nil
}
