package domain

import "testing"

func TestSortReasonsDeterministic(t *testing.T) {
	rs := []Reason{
		{Code: "b", Field: "x"},
		{Code: "a", Field: "z"},
		{Code: "a", Field: "a"},
	}
	SortReasons(rs)
	if rs[0].Code != "a" || rs[0].Field != "a" {
		t.Fatalf("got %+v", rs[0])
	}
	if rs[1].Code != "a" || rs[1].Field != "z" {
		t.Fatalf("got %+v", rs[1])
	}
	if rs[2].Code != "b" {
		t.Fatalf("got %+v", rs[2])
	}
}
