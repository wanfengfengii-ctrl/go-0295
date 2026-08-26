package domain

// TaskStatus is the lifecycle status of a facade task.
type TaskStatus string

const (
	// TaskCreated means the task has been created but not yet locked.
	TaskCreated TaskStatus = "created"
	// TaskLocked means the immutable layout and rules have been fixed.
	TaskLocked TaskStatus = "locked"
	// TaskTerminal means a terminal decision was reached and no further
	// construction or retest progress is permitted.
	TaskTerminal TaskStatus = "terminal"
)

// FacadeTask is the top-level task entity: a building, facade zone and base
// wall type together with the immutable rule snapshot and layout fixed at lock
// time.
type FacadeTask struct {
	ID             string      `json:"id"`
	Building       string      `json:"building"`
	FacadeZone     string      `json:"facade_zone"`
	WallType       string      `json:"wall_type"`
	Status         TaskStatus  `json:"status"`
	Generation     Generation  `json:"generation"`
	SnapshotDigest string      `json:"snapshot_digest"`
	LockedAt       LogicalTime `json:"locked_at,omitempty"`
}
