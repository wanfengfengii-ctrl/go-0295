// Package coverage is the facade task and coverage aggregation component
// ("立面铺贴任务及覆盖聚合"). It locks facade tasks, integer grids and the
// adjacency graph, and maintains append-only board-generation coverage with
// deterministic ordering for geometry-conflict and impact-area queries.
package coverage

import (
	"errors"
	"sort"

	"rockwool-facade-render-handover/internal/domain"
)

var (
	// ErrDegenerate is returned for zero/negative-area (degenerate) geometry.
	ErrDegenerate = errors.New("coverage: degenerate (non-positive area) rectangle")
	// ErrNegative is returned for negative dimensions in integer millimetres.
	ErrNegative = errors.New("coverage: negative dimension")
)

// Point is an integer-millimetre position.
type Point struct {
	X, Y int64
}

// Rect is an axis-aligned integer-millimetre rectangle.
type Rect struct {
	X, Y int64 // origin (top-left)
	W, H int64 // width and height in millimetres
}

// Validate rejects negative dimensions and zero/negative-area (degenerate)
// rectangles, implementing the integer-millimetre geometry boundary.
func (r Rect) Validate() error {
	if r.W < 0 || r.H < 0 {
		return ErrNegative
	}
	if r.W == 0 || r.H == 0 {
		return ErrDegenerate
	}
	return nil
}

// GridCell identifies a target grid cell by integer row and column.
type GridCell struct {
	Row, Col int
}

// Less reports deterministic domain ordering: row-major, then column.
func (c GridCell) Less(o GridCell) bool {
	if c.Row != o.Row {
		return c.Row < o.Row
	}
	return c.Col < o.Col
}

// SortCells sorts cells into deterministic row-major order.
func SortCells(cells []GridCell) {
	sort.SliceStable(cells, func(i, j int) bool { return cells[i].Less(cells[j]) })
}

// CoverageSummary is a deterministic projection digest used for recovery
// checks and lock responses.
type CoverageSummary struct {
	Digest     string
	Generation domain.Generation
}
