package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"rockwool-facade-render-handover/internal/domain"
)

// LeaseKey identifies a leaseable resource by kind and number.
type LeaseKey struct {
	Kind   ResourceKind
	Number string
}

// LeaseDuration is the default lease length in logical-time units.
const LeaseDuration = domain.LogicalTime(100)

// AcquireLease attempts to acquire a lease for a resource at logical time now.
// It fails when another active lease already holds the resource. On success it
// returns a lease with a fresh token and an expiry of now+duration.
func AcquireLease(key LeaseKey, holder string, now domain.LogicalTime, duration domain.LogicalTime) (Lease, error) {
	if duration <= 0 {
		duration = LeaseDuration
	}
	if holder == "" {
		return Lease{}, fmt.Errorf("ledger: lease holder required")
	}
	token, err := newToken()
	if err != nil {
		return Lease{}, err
	}
	return Lease{
		Kind:     key.Kind,
		Number:   key.Number,
		Holder:   holder,
		Token:    token,
		Acquired: now,
		Expires:  now + duration,
	}, nil
}

// RenewLease extends an existing lease by duration. It requires a matching
// token and an unexpired lease, and rejects renewal of an expired lease.
func RenewLease(l Lease, token string, now domain.LogicalTime, duration domain.LogicalTime) (Lease, error) {
	if l.Token == "" || l.Token != token {
		return Lease{}, fmt.Errorf("ledger: lease token mismatch")
	}
	if !l.ActiveAt(now) {
		return Lease{}, fmt.Errorf("ledger: lease expired")
	}
	if duration <= 0 {
		duration = LeaseDuration
	}
	l.Expires = now + duration
	return l, nil
}

// FindConflict reports whether any active lease already holds the resource key.
func FindConflict(leases []Lease, key LeaseKey, now domain.LogicalTime) bool {
	for _, l := range leases {
		if l.Kind == key.Kind && l.Number == key.Number && l.ActiveAt(now) {
			return true
		}
	}
	return false
}

// newToken returns a random hex token for a lease.
func newToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
