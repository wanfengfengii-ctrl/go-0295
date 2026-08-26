package coverage

import (
	"errors"
	"testing"
)

func TestRectValidate(t *testing.T) {
	if err := (Rect{W: 0, H: 10}).Validate(); !errors.Is(err, ErrDegenerate) {
		t.Fatalf("want degenerate, got %v", err)
	}
	if err := (Rect{W: -1, H: 10}).Validate(); !errors.Is(err, ErrNegative) {
		t.Fatalf("want negative, got %v", err)
	}
	if err := (Rect{W: 100, H: 200}).Validate(); err != nil {
		t.Fatalf("valid rect rejected: %v", err)
	}
}

func TestSortCellsDeterministic(t *testing.T) {
	cells := []GridCell{{Row: 1, Col: 2}, {Row: 0, Col: 5}, {Row: 1, Col: 0}}
	SortCells(cells)
	want := []GridCell{{Row: 0, Col: 5}, {Row: 1, Col: 0}, {Row: 1, Col: 2}}
	for i := range want {
		if cells[i] != want[i] {
			t.Fatalf("index %d: got %v want %v", i, cells[i], want[i])
		}
	}
}
