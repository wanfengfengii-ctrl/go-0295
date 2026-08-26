// Package ledger is the mortar conservation and device lease manager
// ("砂浆守恒与设备租约管理器"). It tracks integer-gram material in an
// append-only ledger and enforces time-bounded single-holder device leases,
// completing material withdrawal and lease acquisition atomically.
package ledger

import "rockwool-facade-render-handover/internal/domain"

// ResourceKind enumerates the five leaseable device resource kinds.
type ResourceKind string

const (
	KindMixer       ResourceKind = "mixer"
	KindGlueStation ResourceKind = "glue_station"
	KindDrill       ResourceKind = "drill"
	KindPullTester  ResourceKind = "pull_tester"
	KindProbe       ResourceKind = "probe"
)

// Lease is a time-bounded, single-holder device lease.
type Lease struct {
	Kind     ResourceKind
	Number   string
	Holder   string
	Token    string
	Acquired domain.LogicalTime
	Expires  domain.LogicalTime
}

// ActiveAt reports whether the lease is held by a non-empty token and
// unexpired at logical time t. The boundary excludes t == Expires.
func (l Lease) ActiveAt(t domain.LogicalTime) bool {
	return l.Token != "" && t >= l.Acquired && t < l.Expires
}
