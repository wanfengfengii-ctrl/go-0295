package store

import (
	"encoding/json"
	"fmt"

	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
)

// Engine is the transactional business core. It persists every entity through
// the embedded store and orchestrates the component packages so each command is
// atomic: an error leaves no partial material withdrawal, lease, coverage,
// anchor evidence or terminal credential.
type Engine struct {
	db *DB
	// scripted devices are installed per-command via requests, not stored here.
}

// NewEngine builds an engine over an opened store.
func NewEngine(db *DB) *Engine {
	return &Engine{db: db}
}

// taskLayout is the persisted bundle of the locked layout and its boards.
type taskLayout struct {
	Layout     coverage.Layout           `json:"layout"`
	Boards     []coverage.BoardPlacement `json:"boards"`
	Thresholds catalog.Thresholds        `json:"thresholds"`
}

// CreateTaskRequest requests creation of a new facade task.
type CreateTaskRequest struct {
	Building   string
	FacadeZone string
	WallType   string
}

// CreateTask creates a new, unlocked facade task.
func (e *Engine) CreateTask(req CreateTaskRequest) (domain.FacadeTask, error) {
	if req.Building == "" || req.FacadeZone == "" || req.WallType == "" {
		return domain.FacadeTask{}, &domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Field: "task", Detail: "building, facade zone and wall type required"}}}
	}
	id := taskID(req.Building, req.FacadeZone)
	task := domain.FacadeTask{
		ID:         id,
		Building:   req.Building,
		FacadeZone: req.FacadeZone,
		WallType:   req.WallType,
		Status:     domain.TaskCreated,
		Generation: 1,
	}
	err := e.db.Update(func(tx *Tx) error {
		if ok, _ := tx.GetJSON(BucketTasks, id, &domain.FacadeTask{}); ok {
			return &domain.Failure{Code: domain.CodeOverlap,
				Reasons: []domain.Reason{{Code: domain.CodeOverlap, Field: "task", Detail: "task already exists"}}}
		}
		return tx.PutJSON(BucketTasks, id, task)
	})
	return task, err
}

// LockTaskRequest requests locking a task with immutable rules and layout.
type LockTaskRequest struct {
	Snapshot       catalog.Snapshot
	Thresholds     catalog.Thresholds
	Layout         coverage.Layout
	Boards         []coverage.BoardPlacement
	ExpectedDigest string
}

// LockResult is the deterministic lock response.
type LockResult struct {
	SnapshotDigest string            `json:"snapshot_digest"`
	CoverageDigest string            `json:"coverage_digest"`
	Generation     domain.Generation `json:"generation"`
}

// LockTask locks a task: it validates the snapshot and thresholds, verifies the
// expected digest, validates full non-overlapping coverage, and fixes all
// immutable state atomically.
func (e *Engine) LockTask(id string, req LockTaskRequest) (LockResult, error) {
	if req.ExpectedDigest != "" {
		digest := req.Snapshot.Digest()
		if digest != req.ExpectedDigest {
			return LockResult{}, catalog.StaleDigestError(req.ExpectedDigest, digest)
		}
	}
	if err := req.Snapshot.Validate(); err != nil {
		return LockResult{}, &domain.Failure{Code: domain.CodeDigestMismatch,
			Reasons: []domain.Reason{{Code: domain.CodeDigestMismatch, Detail: err.Error()}}}
	}
	if err := req.Thresholds.Validate(); err != nil {
		return LockResult{}, &domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: err.Error()}}}
	}
	summary, err := coverage.LockLayout(req.Layout, req.Boards)
	if err != nil {
		return LockResult{}, err
	}

	result := LockResult{
		SnapshotDigest: req.Snapshot.Digest(),
		CoverageDigest: summary.Digest,
		Generation:     summary.Generation,
	}
	if result.Generation == 0 {
		result.Generation = 1
	}

	err = e.db.Update(func(tx *Tx) error {
		var task domain.FacadeTask
		ok, err := tx.GetJSON(BucketTasks, id, &task)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(id)
		}
		if task.Status == domain.TaskTerminal {
			return &domain.Failure{Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{Code: domain.CodeTerminalConflict, Detail: "task terminal"}}}
		}
		task.Status = domain.TaskLocked
		task.SnapshotDigest = req.Snapshot.Digest()
		task.Generation = result.Generation
		if err := tx.PutJSON(BucketTasks, id, task); err != nil {
			return err
		}
		if err := tx.PutJSON(BucketSnapshots, id, req.Snapshot); err != nil {
			return err
		}
		if err := tx.PutJSON(BucketLayouts, id, taskLayout{Layout: req.Layout, Boards: req.Boards, Thresholds: req.Thresholds}); err != nil {
			return err
		}
		return tx.AppendEvent([]byte(fmt.Sprintf("lock task=%s digest=%s", id, req.Snapshot.Digest())))
	})
	return result, err
}

// CommandType enumerates the unified command verbs.
type CommandType string

const (
	CommandBase    CommandType = "base"
	CommandMix     CommandType = "mix"
	CommandMortar  CommandType = "mortar"
	CommandGlue    CommandType = "glue"
	CommandPlace   CommandType = "place"
	CommandJoint   CommandType = "joint"
	CommandCure    CommandType = "cure"
	CommandDrill   CommandType = "drill"
	CommandAnchor  CommandType = "anchor"
	CommandInspect CommandType = "inspect"
)

// Command is the unified command payload submitted to the commands endpoint.
type Command struct {
	OperationID string             `json:"operation_id"`
	Type        CommandType        `json:"type"`
	BoardID     string             `json:"board_id,omitempty"`
	Generation  domain.Generation  `json:"generation"`
	LogicalTime domain.LogicalTime `json:"logical_time"`
	LeaseToken  string             `json:"lease_token,omitempty"`

	// mix
	MixerNumber string `json:"mixer_number,omitempty"`
	Holder      string `json:"holder,omitempty"`
	PowderGrams int64  `json:"powder_grams,omitempty"`
	WaterGrams  int64  `json:"water_grams,omitempty"`

	// glue
	GlueGrams int64 `json:"glue_grams,omitempty"`

	// cure
	CureStart   domain.LogicalTime `json:"cure_start,omitempty"`
	CureEnd     domain.LogicalTime `json:"cure_end,omitempty"`
	CureRateNum int64              `json:"cure_rate_num,omitempty"`
	CureRateDen int64              `json:"cure_rate_den,omitempty"`

	// anchor
	AnchorSeq   int   `json:"anchor_seq,omitempty"`
	AnchorX     int64 `json:"anchor_x,omitempty"`
	AnchorY     int64 `json:"anchor_y,omitempty"`
	AnchorDepth int64 `json:"anchor_depth,omitempty"`

	// inspect
	Reading int64 `json:"reading,omitempty"`

	// drill script (deterministic device outcomes)
	DrillScript []evidence.DeviceOutcome `json:"drill_script,omitempty"`
	DrillValues []int64                  `json:"drill_values,omitempty"`
}

// CommandResult is the normalized command response recorded for idempotency.
type CommandResult struct {
	Stage  evidence.Stage `json:"stage,omitempty"`
	Detail string         `json:"detail,omitempty"`
}

// SubmitCommand executes a single command atomically, with idempotency.
func (e *Engine) SubmitCommand(id string, cmd Command) (CommandResult, error) {
	norm, err := json.Marshal(cmd)
	if err != nil {
		return CommandResult{}, err
	}
	normHash := hashBytes(norm)

	var out CommandResult
	err = e.db.Update(func(tx *Tx) error {
		// Idempotency check.
		var rec domain.IdempotencyRecord
		if ok, _ := tx.GetJSON(BucketIdempotency, idempotencyKey(id, cmd.OperationID), &rec); ok {
			if rec.RequestHash != normHash {
				return &domain.Failure{Code: domain.CodeIdempotencyConflict,
					Reasons: []domain.Reason{{Code: domain.CodeIdempotencyConflict, Detail: "different content for operation id"}}}
			}
			return json.Unmarshal([]byte(rec.ResponseHash), &out)
		}

		var task domain.FacadeTask
		ok, err := tx.GetJSON(BucketTasks, id, &task)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(id)
		}
		if task.Status == domain.TaskTerminal {
			return &domain.Failure{Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{Code: domain.CodeTerminalConflict, Detail: "no progress after terminal"}}}
		}
		if cmd.Generation != task.Generation {
			return &domain.Failure{Code: domain.CodeVersionConflict,
				Reasons: []domain.Reason{{Code: domain.CodeVersionConflict, Detail: "generation mismatch"}}}
		}

		res, err := e.dispatch(tx, task, cmd)
		if err != nil {
			return err
		}
		out = res

		raw, _ := json.Marshal(res)
		rec = domain.IdempotencyRecord{
			OperationID:  cmd.OperationID,
			RequestHash:  normHash,
			ResponseHash: string(raw),
			LogicalTime:  cmd.LogicalTime,
		}
		return tx.PutJSON(BucketIdempotency, idempotencyKey(id, cmd.OperationID), rec)
	})
	return out, err
}

// dispatch routes a command to its handler.
func (e *Engine) dispatch(tx *Tx, task domain.FacadeTask, cmd Command) (CommandResult, error) {
	switch cmd.Type {
	case CommandMix:
		return e.doMix(tx, task, cmd)
	case CommandBase, CommandMortar, CommandGlue, CommandPlace, CommandJoint, CommandCure, CommandAnchor, CommandInspect:
		return e.doStage(tx, task, cmd)
	case CommandDrill:
		return e.doDrill(tx, task, cmd)
	default:
		return CommandResult{}, &domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "unknown command type"}}}
	}
}
