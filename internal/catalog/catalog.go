// Package catalog is the insulation construction and material rules catalogue
// ("保温构造与材料规则目录"). It maintains wall, rock wool board, mortar,
// opening wrap, fire break, glue, anchor, curing and inspection rules, and
// produces content-addressed immutable snapshots. Stale summaries and
// facade/summary mismatches are detected by digest, and illegal fixed-point
// thresholds are rejected.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"

	"rockwool-facade-render-handover/internal/domain"
)

var (
	// ErrWallTypeRequired is returned when a snapshot has no wall type.
	ErrWallTypeRequired = errors.New("catalog: wall type is required")
	// ErrBadFixedScale is returned when the fixed-point scale is not positive.
	ErrBadFixedScale = errors.New("catalog: fixed scale must be positive")
)

// Snapshot is the immutable, content-addressed rule snapshot fixed at task
// lock time. Any change to the rules changes the digest.
type Snapshot struct {
	WallType   string
	Materials  map[string]string // material kind -> proof summary
	FixedScale int64             // fixed-point scale factor, must be > 0
	Sampling   map[string]string // sampling mapping entries
	CreatedAt  domain.LogicalTime
}

// Digest returns the content-addressed SHA-256 hex digest of the snapshot.
// Map keys are sorted so logically equal snapshots always yield equal digests.
func (s Snapshot) Digest() string {
	h := sha256.New()
	h.Write([]byte(s.WallType))
	h.Write([]byte{0})
	h.Write([]byte("scale="))
	h.Write([]byte(strconv.FormatInt(s.FixedScale, 10)))
	h.Write([]byte{0})
	writeMap := func(m map[string]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{'='})
			h.Write([]byte(m[k]))
			h.Write([]byte{0})
		}
	}
	writeMap(s.Materials)
	h.Write([]byte{0})
	writeMap(s.Sampling)
	return hex.EncodeToString(h.Sum(nil))
}

// Validate rejects missing wall type and illegal fixed-point thresholds.
func (s Snapshot) Validate() error {
	if s.WallType == "" {
		return ErrWallTypeRequired
	}
	if s.FixedScale <= 0 {
		return ErrBadFixedScale
	}
	return nil
}
