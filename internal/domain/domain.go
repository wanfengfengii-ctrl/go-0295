// Package domain holds the shared, stable domain types used across the service:
// logical time, task generation, stable error codes, the unified failure shape
// and the idempotency record.
package domain

import "sort"

// LogicalTime is a strictly monotonic logical clock value used to order every
// operation. It is independent of wall-clock time so behaviour is deterministic
// and tests never depend on real time or random backoff.
type LogicalTime int64

// Generation identifies the immutable generation of a task layout or retest
// round. Higher generations supersede lower ones; history is never deleted.
type Generation int64

// ErrorCode is a stable, machine-readable rejection category.
type ErrorCode string

const (
	CodeInvalid             ErrorCode = "invalid"
	CodeStaleDigest         ErrorCode = "stale_digest"
	CodeDigestMismatch      ErrorCode = "digest_mismatch"
	CodeOverlap             ErrorCode = "coverage_overlap"
	CodeMissingCell         ErrorCode = "coverage_missing"
	CodeDegenerate          ErrorCode = "coverage_degenerate"
	CodeVerticalJoint       ErrorCode = "vertical_joint"
	CodeCornerOpening       ErrorCode = "corner_opening"
	CodeIdempotencyConflict ErrorCode = "idempotency_conflict"
	CodeLeaseExpired        ErrorCode = "lease_expired"
	CodeLeaseBusy           ErrorCode = "lease_busy"
	CodeConservation        ErrorCode = "mass_conservation"
	CodeOpenExpired         ErrorCode = "open_time_expired"
	CodePrefixViolation     ErrorCode = "prefix_violation"
	CodeArithmeticOverflow  ErrorCode = "arithmetic_overflow"
	CodeRecovery            ErrorCode = "recovery_inconsistent"
	CodeTerminalConflict    ErrorCode = "terminal_conflict"
	CodeNotFound            ErrorCode = "not_found"
	CodeVersionConflict     ErrorCode = "version_conflict"
	CodeDuplicateAnchor     ErrorCode = "duplicate_anchor"
	CodeAnchorEdge          ErrorCode = "anchor_edge_violation"
	CodeAnchorSpacing       ErrorCode = "anchor_spacing"
	CodeAnchorInterference  ErrorCode = "anchor_interference"
	CodeStagger             ErrorCode = "stagger_violation"
	CodeWaterRatio          ErrorCode = "water_ratio"
	CodeCuringGap           ErrorCode = "curing_gap"
	CodeRetestIncomplete    ErrorCode = "retest_incomplete"
	CodeReviewInsufficient  ErrorCode = "review_insufficient"
	CodeLateReceipt         ErrorCode = "late_receipt"
	CodeDeviceError         ErrorCode = "device_error"
)

// Reason describes a single ordered validation failure.
type Reason struct {
	Code   ErrorCode `json:"code"`
	Field  string    `json:"field,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

// Failure is the unified rejection structure returned by the HTTP API.
type Failure struct {
	Code      ErrorCode `json:"code"`
	Reasons   []Reason  `json:"reasons"`
	Retryable bool      `json:"retryable"`
}

// Error makes Failure implement the error interface, so domain failures can be
// returned directly from transactional operations.
func (f *Failure) Error() string {
	if f == nil {
		return "<nil>"
	}
	return "domain failure: " + string(f.Code)
}

// SortReasons orders reasons deterministically by code, then field, then
// detail, so identical inputs always produce identical reason sequences.
func SortReasons(rs []Reason) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Code != rs[j].Code {
			return rs[i].Code < rs[j].Code
		}
		if rs[i].Field != rs[j].Field {
			return rs[i].Field < rs[j].Field
		}
		return rs[i].Detail < rs[j].Detail
	})
}

// IdempotencyRecord stores the normalized request digest and response digest
// for an operation number, enabling idempotent command handling.
type IdempotencyRecord struct {
	OperationID  string      `json:"operation_id"`
	RequestHash  string      `json:"request_hash"`
	ResponseHash string      `json:"response_hash"`
	LogicalTime  LogicalTime `json:"logical_time"`
}
