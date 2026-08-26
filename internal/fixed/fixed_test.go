package fixed

import (
	"errors"
	"math"
	"testing"
)

func TestMulCheckedOverflow(t *testing.T) {
	if _, err := MulChecked(math.MaxInt64, 2); !errors.Is(err, ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
	if _, err := MulChecked(math.MinInt64, -1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
	if v, err := MulChecked(6, 7); err != nil || v != 42 {
		t.Fatalf("got %d, %v", v, err)
	}
}

func TestDivChecked(t *testing.T) {
	if _, err := DivChecked(10, 0); !errors.Is(err, ErrDivZero) {
		t.Fatalf("want div zero, got %v", err)
	}
	if _, err := DivChecked(math.MinInt64, -1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
	if v, err := DivChecked(10, 2); err != nil || v != 5 {
		t.Fatalf("got %d, %v", v, err)
	}
}

func TestMulDivRounding(t *testing.T) {
	// (7*1000)/2 = 3500 exactly.
	if got, err := MulDivChecked(7, 1000, 2); err != nil || got != 3500 {
		t.Fatalf("got %d, %v", got, err)
	}
	// 5/2 rounds half away from zero to 3.
	if got, err := DivRoundChecked(5, 2); err != nil || got != 3 {
		t.Fatalf("got %d, %v", got, err)
	}
	// -5/2 rounds half away from zero to -3.
	if got, err := DivRoundChecked(-5, 2); err != nil || got != -3 {
		t.Fatalf("got %d, %v", got, err)
	}
	// (MaxInt64*2)/1 must overflow before the divide.
	if _, err := MulDivChecked(math.MaxInt64, 2, 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
}

func TestNonNegative(t *testing.T) {
	if _, err := NonNegative(-1); !errors.Is(err, ErrNegative) {
		t.Fatalf("want negative, got %v", err)
	}
	if v, err := NonNegative(5); err != nil || v != 5 {
		t.Fatalf("got %d, %v", v, err)
	}
}
