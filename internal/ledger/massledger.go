package ledger

import (
	"fmt"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/fixed"
)

// MortarState is the integer-gram conservation pool for one mortar mixing
// generation. Every gram is accounted: withdrawn powder and water must equal
// sample plus waste plus attributed glue plus unused remainder.
type MortarState struct {
	Batch      string
	Generation domain.Generation
	Powder     int64
	Water      int64
	Sample     int64
	Waste      int64
	Remainder  int64
	Glue       map[string]int64 // board id -> attributed glue grams
	OpenUntil  domain.LogicalTime
}

// NewMortarState initialises an empty conservation pool for a batch.
func NewMortarState(batch string, generation domain.Generation) *MortarState {
	return &MortarState{
		Batch:      batch,
		Generation: generation,
		Glue:       map[string]int64{},
	}
}

// GlueTotal returns the sum of all attributed glue.
func (m *MortarState) GlueTotal() int64 {
	var total int64
	for _, g := range m.Glue {
		total += g
	}
	return total
}

// CheckConservation verifies the integer-gram conservation invariant:
// powder + water == sample + waste + remainder + glue.
func (m *MortarState) CheckConservation() bool {
	return m.Powder+m.Water == m.Sample+m.Waste+m.Remainder+m.GlueTotal()
}

// WaterRatio checks that water/powder equals num/den using integer arithmetic
// (no floating point). It reports false on a zero denominator or negative
// amounts.
func (m *MortarState) WaterRatio(num, den int64) bool {
	if den <= 0 || m.Powder <= 0 {
		return false
	}
	// water/powder == num/den  <=>  water*den == num*powder (checked)
	lhs, okL := mul(m.Water, den)
	rhs, okR := mul(num, m.Powder)
	if !okL || !okR {
		return false
	}
	return lhs == rhs
}

// Withdraw adds powder and water to the pool and recomputes the remainder so
// the pool starts in conservation. It returns an error when amounts are
// negative or the water ratio is violated.
func (m *MortarState) Withdraw(powder, water int64, ratioNum, ratioDen int64) error {
	if _, err := fixed.NonNegative(powder); err != nil {
		return fmt.Errorf("ledger: powder %w", err)
	}
	if _, err := fixed.NonNegative(water); err != nil {
		return fmt.Errorf("ledger: water %w", err)
	}
	if powder == 0 {
		return fmt.Errorf("ledger: powder withdrawal must be positive")
	}
	m.Powder += powder
	m.Water += water
	if !m.WaterRatio(ratioNum, ratioDen) {
		m.Powder -= powder
		m.Water -= water
		return fmt.Errorf("ledger: water ratio violation")
	}
	m.Remainder = m.Powder + m.Water - m.Sample - m.Waste - m.GlueTotal()
	return nil
}

// ConsumeGlue attributes glue grams to a board out of the remainder. It
// rejects over-consumption (which would break conservation).
func (m *MortarState) ConsumeGlue(board string, grams int64) error {
	if grams < 0 {
		return fmt.Errorf("ledger: glue amount must be non-negative")
	}
	if grams > m.Remainder {
		return fmt.Errorf("ledger: glue over-consumption")
	}
	m.Glue[board] += grams
	m.Remainder -= grams
	return nil
}

// AddWaste moves grams from the remainder into the waste bucket.
func (m *MortarState) AddWaste(grams int64) error {
	if grams < 0 {
		return fmt.Errorf("ledger: waste amount must be non-negative")
	}
	if grams > m.Remainder {
		return fmt.Errorf("ledger: waste over-allocation")
	}
	m.Waste += grams
	m.Remainder -= grams
	return nil
}

// AddSample moves grams from the remainder into the sample bucket.
func (m *MortarState) AddSample(grams int64) error {
	if grams < 0 {
		return fmt.Errorf("ledger: sample amount must be non-negative")
	}
	if grams > m.Remainder {
		return fmt.Errorf("ledger: sample over-allocation")
	}
	m.Sample += grams
	m.Remainder -= grams
	return nil
}

// mul is a small overflow-checked multiply for ratio comparisons.
func mul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	r := a * b
	if r/b != a {
		return 0, false
	}
	return r, true
}
