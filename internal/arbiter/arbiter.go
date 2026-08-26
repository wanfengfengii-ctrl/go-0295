// Package arbiter is the pull-out retest and terminal arbitrator
// ("拉拔复验与终局仲裁器"). It executes sampling mapping, integer fixed-point
// strength calculations, anomaly propagation, retest generations, two-person
// review and the single-writer terminal competition that produces the unique
// handover credential.
//
// The concrete retest logic lives in retest.go and the terminal competition in
// terminal.go; this file declares the terminal kind enum and the component
// contract.
package arbiter

// TerminalKind enumerates the three mutually-exclusive terminal outcomes.
type TerminalKind string

const (
	TerminalHandover   TerminalKind = "handover"
	TerminalQuarantine TerminalKind = "quarantine"
	TerminalCancel     TerminalKind = "cancel"
)
