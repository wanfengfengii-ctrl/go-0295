package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
	"rockwool-facade-render-handover/internal/fixed"
	"rockwool-facade-render-handover/internal/ledger"
)

// doMix prepares a mortar mixing generation: it atomically acquires a mixer
// lease and withdraws integer grams of powder and water, verifying the water
// ratio. If the mixer is busy the whole operation rolls back, leaving no
// material withdrawal and no lease.
func (e *Engine) doMix(tx *Tx, task domain.FacadeTask, cmd Command) (CommandResult, error) {
	layout, err := loadLayout(tx, task.ID)
	if err != nil {
		return CommandResult{}, err
	}
	key := ledger.LeaseKey{Kind: ledger.KindMixer, Number: cmd.MixerNumber}
	var existing []ledger.Lease
	_ = tx.ForEach(BucketLeases, func() any { return &ledger.Lease{} }, func(k string, v any) error {
		l := v.(*ledger.Lease)
		if l.Kind == ledger.KindMixer {
			existing = append(existing, *l)
		}
		return nil
	})
	if ledger.FindConflict(existing, key, cmd.LogicalTime) {
		return CommandResult{}, &domain.Failure{Code: domain.CodeLeaseBusy,
			Reasons: []domain.Reason{{Code: domain.CodeLeaseBusy, Detail: "mixer busy"}}}
	}
	lease, err := ledger.AcquireLease(key, cmd.Holder, cmd.LogicalTime, ledger.LeaseDuration)
	if err != nil {
		return CommandResult{}, &domain.Failure{Code: domain.CodeInvalid, Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: err.Error()}}}
	}

	mortar, err := loadMortar(tx, task.ID)
	if err != nil {
		return CommandResult{}, err
	}
	if err := mortar.Withdraw(cmd.PowderGrams, cmd.WaterGrams,
		layout.Thresholds.WaterRatioNum, layout.Thresholds.WaterRatioDen); err != nil {
		return CommandResult{}, &domain.Failure{Code: domain.CodeWaterRatio,
			Reasons: []domain.Reason{{Code: domain.CodeWaterRatio, Detail: err.Error()}}}
	}

	if err := tx.PutJSON(BucketLeases, leaseKey(task.ID, lease.Kind, lease.Number), lease); err != nil {
		return CommandResult{}, err
	}
	if err := tx.PutJSON(BucketMortar, task.ID, mortar); err != nil {
		return CommandResult{}, err
	}
	return CommandResult{Detail: "mortar mixed"}, nil
}

// doStage advances a single board through the locked stage machine.
func (e *Engine) doStage(tx *Tx, task domain.FacadeTask, cmd Command) (CommandResult, error) {
	if cmd.BoardID == "" {
		return CommandResult{}, &domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "board id required"}}}
	}
	layout, err := loadLayout(tx, task.ID)
	if err != nil {
		return CommandResult{}, err
	}
	board := findBoard(layout.Boards, cmd.BoardID)
	if board == nil {
		return CommandResult{}, notFound(cmd.BoardID)
	}

	rec, err := loadBoard(tx, task.ID, cmd.BoardID, cmd.Generation)
	if err != nil {
		return CommandResult{}, err
	}

	var to evidence.Stage
	switch cmd.Type {
	case CommandBase:
		to = evidence.StageBaseAccepted
	case CommandMortar:
		to = evidence.StageMortarValid
	case CommandGlue:
		to = evidence.StageGluePrefix
	case CommandPlace:
		to = evidence.StagePlaced
	case CommandJoint:
		to = evidence.StageJoint
	case CommandCure:
		to = evidence.StageCured
	case CommandAnchor:
		to = evidence.StageAnchored
	case CommandInspect:
		to = evidence.StageInspected
	}

	if cmd.Type == CommandGlue {
		if err := e.consumeGlue(tx, task, cmd, board); err != nil {
			return CommandResult{}, err
		}
	}
	if cmd.Type == CommandCure {
		if err := e.addCuring(tx, task, cmd, board); err != nil {
			return CommandResult{}, err
		}
	}
	if cmd.Type == CommandAnchor {
		if err := e.addAnchor(tx, task, cmd, board, layout); err != nil {
			return CommandResult{}, err
		}
	}

	updated, err := evidence.Advance(rec, evidence.AdvanceRequest{
		BoardID:     cmd.BoardID,
		Generation:  cmd.Generation,
		From:        rec.Stage,
		To:          to,
		LogicalTime: cmd.LogicalTime,
	})
	if err != nil {
		return CommandResult{}, &domain.Failure{Code: domain.CodePrefixViolation,
			Reasons: []domain.Reason{{Code: domain.CodePrefixViolation, Detail: err.Error()}}}
	}
	if err := tx.PutJSON(BucketStages, boardKey(task.ID, cmd.BoardID), updated); err != nil {
		return CommandResult{}, err
	}
	return CommandResult{Stage: updated.Stage}, nil
}

// consumeGlue attributes glue grams to a board out of the mortar remainder,
// validating the required glue from board area and unit rate using checked
// integer arithmetic.
func (e *Engine) consumeGlue(tx *Tx, task domain.FacadeTask, cmd Command, board *coverage.BoardPlacement) error {
	area, err := fixed.MulChecked(int64(board.Rows*board.Cols), cellAreaMM2())
	if err != nil {
		return &domain.Failure{Code: domain.CodeArithmeticOverflow,
			Reasons: []domain.Reason{{Code: domain.CodeArithmeticOverflow, Detail: "board area overflow"}}}
	}
	layout, _ := loadLayout(tx, task.ID)
	required, err := fixed.MulDivChecked(area, layout.Thresholds.UnitAreaGlue, layout.Thresholds.FixedScale)
	if err != nil {
		return &domain.Failure{Code: domain.CodeArithmeticOverflow,
			Reasons: []domain.Reason{{Code: domain.CodeArithmeticOverflow, Detail: err.Error()}}}
	}
	if required > 0 && cmd.GlueGrams < required {
		return &domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "insufficient glue for board area"}}}
	}
	mortar, err := loadMortar(tx, task.ID)
	if err != nil {
		return err
	}
	if err := mortar.ConsumeGlue(cmd.BoardID, cmd.GlueGrams); err != nil {
		return &domain.Failure{Code: domain.CodeConservation,
			Reasons: []domain.Reason{{Code: domain.CodeConservation, Detail: err.Error()}}}
	}
	return tx.PutJSON(BucketMortar, task.ID, mortar)
}

// addCuring appends a curing interval and verifies continuity plus the minimum
// equivalent curing time.
func (e *Engine) addCuring(tx *Tx, task domain.FacadeTask, cmd Command, board *coverage.BoardPlacement) error {
	iv := evidence.CuringInterval{
		Start: cmd.CureStart, End: cmd.CureEnd,
		RateNum: cmd.CureRateNum, RateDen: cmd.CureRateDen,
	}
	if iv.RateDen == 0 {
		iv.RateDen = 1
	}
	if iv.RateNum == 0 {
		iv.RateNum = 1
	}
	intervals, err := loadCuring(tx, task.ID, cmd.BoardID)
	if err != nil {
		return err
	}
	intervals = append(intervals, iv)
	if err := evidence.CheckContinuity(intervals); err != nil {
		return &domain.Failure{Code: domain.CodeCuringGap,
			Reasons: []domain.Reason{{Code: domain.CodeCuringGap, Detail: err.Error()}}}
	}
	total, err := evidence.IntegrateCuring(intervals)
	if err != nil {
		return &domain.Failure{Code: domain.CodeArithmeticOverflow,
			Reasons: []domain.Reason{{Code: domain.CodeArithmeticOverflow, Detail: err.Error()}}}
	}
	layout, _ := loadLayout(tx, task.ID)
	if total < layout.Thresholds.MinCureSecs {
		return &domain.Failure{Code: domain.CodeCuringGap,
			Reasons: []domain.Reason{{Code: domain.CodeCuringGap, Detail: "curing time below minimum"}}}
	}
	return tx.PutJSON(BucketCuring, boardKey(task.ID, cmd.BoardID), intervals)
}

// addAnchor installs one anchor after validating the sequence, edge margin,
// spacing and depth rules, and recording a scripted depth measurement.
func (e *Engine) addAnchor(tx *Tx, task domain.FacadeTask, cmd Command, board *coverage.BoardPlacement, layout *taskLayout) error {
	rect := coverage.Rect{
		X: int64(board.Col) * cellMM(),
		Y: int64(board.Row) * cellMM(),
		W: int64(board.Cols) * cellMM(),
		H: int64(board.Rows) * cellMM(),
	}
	anchors, err := loadAnchors(tx, task.ID, cmd.BoardID)
	if err != nil {
		return err
	}
	next := evidence.Anchor{
		Seq:     cmd.AnchorSeq,
		Hole:    coverage.Point{X: cmd.AnchorX, Y: cmd.AnchorY},
		DepthMM: cmd.AnchorDepth,
	}
	if err := evidence.ValidateAnchor(anchors, next, rect,
		layout.Thresholds.MinEdgeMM, layout.Thresholds.MinSpacingMM, layout.Thresholds.MinPullStrength); err != nil {
		code := domain.CodeAnchorEdge
		if cmd.AnchorSeq != len(anchors)+1 {
			code = domain.CodePrefixViolation
		}
		return &domain.Failure{Code: code, Reasons: []domain.Reason{{Code: code, Detail: err.Error()}}}
	}
	anchors = append(anchors, next)
	return tx.PutJSON(BucketAnchors, boardKey(task.ID, cmd.BoardID), anchors)
}

// doDrill runs the scripted depth meter through its deterministic outcomes,
// recording pending-retry calls for failures and only writing a real reading
// (and advancing to allow anchoring) on success.
func (e *Engine) doDrill(tx *Tx, task domain.FacadeTask, cmd Command) (CommandResult, error) {
	device := evidence.NewScriptedDevice(cmd.DrillScript, cmd.DrillValues)
	var calls []evidence.DeviceCall
	// The device is driven by its script; each non-success outcome only forms a
	// pending-retry record and never produces a reading or advances state.
	for attempt := 1; attempt <= evidence.RetryLimit; attempt++ {
		outcome, value := device.Call()
		call := evidence.DeviceCall{
			Device:      "depth_meter",
			Attempt:     attempt,
			Outcome:     outcome,
			FaultCode:   evidence.FaultCodeFor(outcome),
			LogicalTime: cmd.LogicalTime,
			Value:       value,
			RawValid:    outcome == evidence.OutcomeSuccess,
		}
		calls = append(calls, call)
		if outcome == evidence.OutcomeSuccess {
			return CommandResult{Detail: fmt.Sprintf("drilled depth=%d", value)},
				tx.PutJSON(BucketInspections, boardKey(task.ID, cmd.BoardID), calls)
		}
		if outcome == evidence.OutcomeDisconnect {
			// Script exhausted without success.
			break
		}
	}
	return CommandResult{}, &domain.Failure{Code: domain.CodeDeviceError,
		Reasons: []domain.Reason{{Code: domain.CodeDeviceError, Detail: "retries exhausted"}}}
}

// helpers ---------------------------------------------------------------------

func taskID(building, zone string) string {
	return fmt.Sprintf("%s/%s", building, zone)
}

func leaseKey(taskID string, kind ledger.ResourceKind, number string) string {
	return keyJoin(taskID, string(kind), number)
}

func boardKey(taskID, boardID string) string { return keyJoin(taskID, boardID) }

// idempotencyKey scopes an operation id to its task. Boards, leases and reviews
// are already keyed per task; idempotency was the lone global key, so the same
// operation id reused across two tasks short-circuited the second task's command
// (same normalized bytes) and returned the first task's cached response without
// ever running dispatch, leaving the second task's board in its prior stage.
func idempotencyKey(taskID, operationID string) string { return keyJoin(taskID, operationID) }

func cellMM() int64 { return 1000 }

func cellAreaMM2() int64 { return cellMM() * cellMM() }

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func notFound(id string) error {
	return &domain.Failure{Code: domain.CodeNotFound,
		Reasons: []domain.Reason{{Code: domain.CodeNotFound, Detail: id}}}
}

func loadLayout(tx *Tx, taskID string) (*taskLayout, error) {
	var tl taskLayout
	ok, err := tx.GetJSON(BucketLayouts, taskID, &tl)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notFound(taskID)
	}
	return &tl, nil
}

func findBoard(boards []coverage.BoardPlacement, id string) *coverage.BoardPlacement {
	for i := range boards {
		if boards[i].ID == id {
			return &boards[i]
		}
	}
	return nil
}

func loadMortar(tx *Tx, taskID string) (*ledger.MortarState, error) {
	m := ledger.NewMortarState("", 0)
	ok, err := tx.GetJSON(BucketMortar, taskID, m)
	if err != nil {
		return nil, err
	}
	if !ok {
		return m, nil
	}
	return m, nil
}

func loadBoard(tx *Tx, taskID, boardID string, gen domain.Generation) (evidence.BoardRecord, error) {
	rec := evidence.BoardRecord{BoardID: boardID, Generation: gen, Stage: evidence.StageNone}
	ok, err := tx.GetJSON(BucketStages, boardKey(taskID, boardID), &rec)
	if err != nil {
		return rec, err
	}
	if !ok {
		return rec, nil
	}
	return rec, nil
}

func loadCuring(tx *Tx, taskID, boardID string) ([]evidence.CuringInterval, error) {
	var out []evidence.CuringInterval
	ok, err := tx.GetJSON(BucketCuring, boardKey(taskID, boardID), &out)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return out, nil
}

func loadAnchors(tx *Tx, taskID, boardID string) ([]evidence.Anchor, error) {
	var out []evidence.Anchor
	ok, err := tx.GetJSON(BucketAnchors, boardKey(taskID, boardID), &out)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return out, nil
}
