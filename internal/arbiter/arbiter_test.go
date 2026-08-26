package arbiter

import "testing"

func TestMeetsReviewQuorum(t *testing.T) {
	same := []Review{
		{Reviewer: "alice", Qualified: true, Opinion: "approve"},
		{Reviewer: "alice", Qualified: true, Opinion: "approve"},
	}
	if MeetsReviewQuorum(same) {
		t.Fatal("same reviewer twice must not satisfy two-person quorum")
	}
	two := []Review{
		{Reviewer: "alice", Qualified: true, Opinion: "approve"},
		{Reviewer: "bob", Qualified: true, Opinion: "approve"},
	}
	if !MeetsReviewQuorum(two) {
		t.Fatal("two distinct qualified reviewers must satisfy quorum")
	}
	unqualified := []Review{
		{Reviewer: "alice", Qualified: false, Opinion: "approve"},
		{Reviewer: "bob", Qualified: true, Opinion: "approve"},
	}
	if MeetsReviewQuorum(unqualified) {
		t.Fatal("unqualified reviewer must not count")
	}
}
