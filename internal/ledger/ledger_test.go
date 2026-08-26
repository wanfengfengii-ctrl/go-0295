package ledger

import "testing"

func TestLeaseActiveAt(t *testing.T) {
	l := Lease{Token: "tok", Acquired: 10, Expires: 20}
	if !l.ActiveAt(10) {
		t.Fatal("active at acquire time")
	}
	if !l.ActiveAt(19) {
		t.Fatal("active before expiry")
	}
	if l.ActiveAt(20) {
		t.Fatal("must be expired at boundary")
	}
	if l.ActiveAt(9) {
		t.Fatal("must be inactive before acquire")
	}
	if (Lease{Token: "", Acquired: 10, Expires: 20}).ActiveAt(15) {
		t.Fatal("empty token lease must be inactive")
	}
}

func TestResourceKinds(t *testing.T) {
	kinds := []ResourceKind{KindMixer, KindGlueStation, KindDrill, KindPullTester, KindProbe}
	for _, k := range kinds {
		if k == "" {
			t.Fatal("empty resource kind")
		}
	}
}
