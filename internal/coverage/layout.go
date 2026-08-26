package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"rockwool-facade-render-handover/internal/domain"
)

// Cell is a target grid cell by integer row and column.
type Cell struct {
	Row, Col int
}

// Corner is a grid-line coordinate where a board corner may sit. Corners are
// used to express opening corner forbidden zones.
type Corner struct {
	Row, Col int
}

// BoardPlacement places a rectangular board over a contiguous block of cells.
// The board occupies cells from (Row,Col) to (Row+Rows-1, Col+Cols-1).
type BoardPlacement struct {
	ID         string
	Row, Col   int
	Rows, Cols int
	Generation domain.Generation
	Material   string
	BaseZone   string
}

// cells returns every cell covered by the board.
func (b BoardPlacement) cells() []Cell {
	out := make([]Cell, 0, b.Rows*b.Cols)
	for r := b.Row; r < b.Row+b.Rows; r++ {
		for c := b.Col; c < b.Col+b.Cols; c++ {
			out = append(out, Cell{Row: r, Col: c})
		}
	}
	return out
}

// Layout describes the locked facade geometry: the target grid, opening cells,
// forbidden corner points and explicit adjacency edges. Opening and forbidden
// corner sets use string keys so the layout is JSON-serialisable for
// persistence.
type Layout struct {
	Rows, Cols       int
	Openings         map[string]bool // "row,col" -> true
	ForbiddenCorners map[string]bool // "row,col" -> true
	Adjacency        []AdjEdge
	BaseZones        map[string]int // zone name -> index, for deterministic ordering
	MortarBatches    []string
}

// CellKey returns the canonical string key for a cell.
func CellKey(c Cell) string { return fmt.Sprintf("%d,%d", c.Row, c.Col) }

// CornerKey returns the canonical string key for a corner.
func CornerKey(c Corner) string { return fmt.Sprintf("%d,%d", c.Row, c.Col) }

// AdjEdge is a public adjacency relationship between two board ids.
type AdjEdge struct {
	A, B string
}

// LockLayout validates that boards fully cover the target grid without illegal
// overlap, degenerate regions, vertical through joints or opening-corner seams,
// and returns the deterministic coverage summary.
func LockLayout(layout Layout, boards []BoardPlacement) (CoverageSummary, error) {
	var reasons []domain.Reason
	reasons = append(reasons, validateGeometry(layout, boards)...)
	if len(reasons) > 0 {
		domain.SortReasons(reasons)
		return CoverageSummary{}, &domain.Failure{
			Code:    domain.CodeInvalid,
			Reasons: reasons,
		}
	}
	digest := summarize(boards)
	return CoverageSummary{Digest: digest, Generation: currentGeneration(boards)}, nil
}

// currentGeneration returns the maximum board generation (the effective one).
func currentGeneration(boards []BoardPlacement) domain.Generation {
	var g domain.Generation
	for _, b := range boards {
		if b.Generation > g {
			g = b.Generation
		}
	}
	return g
}

// validateGeometry performs the integer-millimetre geometry boundary checks and
// returns ordered reasons. The empty result means the layout is valid.
func validateGeometry(layout Layout, boards []BoardPlacement) []domain.Reason {
	var reasons []domain.Reason

	// Degenerate boards and negative dimensions.
	for _, b := range boards {
		if b.Rows <= 0 || b.Cols <= 0 {
			reasons = append(reasons, domain.Reason{
				Code:   domain.CodeDegenerate,
				Field:  "board",
				Detail: b.ID,
			})
		}
		if b.Row < 0 || b.Col < 0 {
			reasons = append(reasons, domain.Reason{
				Code:   domain.CodeDegenerate,
				Field:  "board",
				Detail: fmt.Sprintf("%s negative origin", b.ID),
			})
		}
	}

	// Build a cell -> owner map to detect overlap and missing cells.
	owner := map[Cell]string{}
	overlap := map[Cell]bool{}
	for _, b := range boards {
		for _, c := range b.cells() {
			if prev, ok := owner[c]; ok && prev != b.ID {
				overlap[c] = true
			} else {
				owner[c] = b.ID
			}
		}
	}
	for c := range overlap {
		reasons = append(reasons, domain.Reason{
			Code:   domain.CodeOverlap,
			Field:  "cell",
			Detail: fmt.Sprintf("row=%d col=%d", c.Row, c.Col),
		})
	}

	// Missing target cells and boards covering opening cells.
	for r := 0; r < layout.Rows; r++ {
		for c := 0; c < layout.Cols; c++ {
			cell := Cell{Row: r, Col: c}
			if layout.Openings[CellKey(cell)] {
				if _, covered := owner[cell]; covered {
					reasons = append(reasons, domain.Reason{
						Code:   domain.CodeOverlap,
						Field:  "opening",
						Detail: fmt.Sprintf("row=%d col=%d", r, c),
					})
				}
				continue
			}
			if _, covered := owner[cell]; !covered {
				reasons = append(reasons, domain.Reason{
					Code:   domain.CodeMissingCell,
					Field:  "cell",
					Detail: fmt.Sprintf("row=%d col=%d", r, c),
				})
			}
		}
	}

	// Vertical through joints: an interior vertical seam position shared by two
	// consecutive courses. Facade boundary edges (where only one board touches)
	// are not seams and therefore never flagged.
	seamsByRow := map[int]map[int]bool{}
	for _, b := range boards {
		if seamsByRow[b.Row] == nil {
			seamsByRow[b.Row] = map[int]bool{}
		}
	}
	// A seam exists at column boundary x when one board's right edge equals x
	// and another board's left edge equals x in the same row.
	for _, b := range boards {
		for _, other := range boards {
			if b.Row != other.Row || b.ID == other.ID {
				continue
			}
			if b.Col+b.Cols == other.Col {
				seamsByRow[b.Row][b.Col+b.Cols] = true
			}
		}
	}
	for _, b := range boards {
		next := b.Row + b.Rows
		if seams, ok := seamsByRow[next]; ok {
			right := b.Col + b.Cols
			if seams[right] {
				reasons = append(reasons, domain.Reason{
					Code:   domain.CodeVerticalJoint,
					Field:  "board",
					Detail: fmt.Sprintf("%s seam col=%d", b.ID, right),
				})
			}
		}
	}

	// Opening-corner forbidden seams: a board corner landing on a forbidden
	// corner point.
	for _, b := range boards {
		corners := []Corner{
			{Row: b.Row, Col: b.Col},
			{Row: b.Row, Col: b.Col + b.Cols},
			{Row: b.Row + b.Rows, Col: b.Col},
			{Row: b.Row + b.Rows, Col: b.Col + b.Cols},
		}
		for _, c := range corners {
			if layout.ForbiddenCorners[CornerKey(c)] {
				reasons = append(reasons, domain.Reason{
					Code:   domain.CodeCornerOpening,
					Field:  "board",
					Detail: fmt.Sprintf("%s corner row=%d col=%d", b.ID, c.Row, c.Col),
				})
			}
		}
	}

	return reasons
}

// summarize produces a deterministic content-addressed digest over the sorted
// board placements. It is used for recovery checks and lock responses.
func summarize(boards []BoardPlacement) string {
	sorted := make([]BoardPlacement, len(boards))
	copy(sorted, boards)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	h := sha256.New()
	for _, b := range sorted {
		fmt.Fprintf(h, "%s|%d|%d|%d|%d|%d|%s|%s\n",
			b.ID, b.Row, b.Col, b.Rows, b.Cols, b.Generation, b.Material, b.BaseZone)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// CoverageDigest exposes the deterministic digest computation for recovery
// verification without performing full validation.
func CoverageDigest(boards []BoardPlacement) string { return summarize(boards) }
