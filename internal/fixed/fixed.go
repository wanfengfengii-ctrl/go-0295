// Package fixed implements checked signed-integer fixed-point arithmetic used
// for every domain calculation (lengths in millimetres, masses in grams and
// scaled ratios/strengths). It enforces the project invariant that multiply,
// divide, add, subtract and rounding first reject negative values where
// required, division by zero and overflow, and that no floating-point value
// ever participates in a ruling.
package fixed

import (
	"errors"
	"math"
)

var (
	// ErrOverflow is returned when an operation would exceed the int64 range.
	ErrOverflow = errors.New("fixed: integer overflow")
	// ErrDivZero is returned when a division or mul-div would divide by zero.
	ErrDivZero = errors.New("fixed: division by zero")
	// ErrNegative is returned when a negative value is not permitted.
	ErrNegative = errors.New("fixed: negative value not permitted")
)

// AddChecked returns a+b or ErrOverflow.
func AddChecked(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// MulChecked returns a*b or ErrOverflow.
func MulChecked(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, ErrOverflow
	}
	r := a * b
	if r/b != a {
		return 0, ErrOverflow
	}
	return r, nil
}

// DivChecked returns a/b or ErrDivZero / ErrOverflow.
func DivChecked(a, b int64) (int64, error) {
	if b == 0 {
		return 0, ErrDivZero
	}
	if a == math.MinInt64 && b == -1 {
		return 0, ErrOverflow
	}
	return a / b, nil
}

// DivRoundChecked returns a/b rounded half away from zero (the locked rounding
// rule), or ErrDivZero / ErrOverflow.
func DivRoundChecked(a, b int64) (int64, error) {
	if b == 0 {
		return 0, ErrDivZero
	}
	if a == math.MinInt64 && b == -1 {
		return 0, ErrOverflow
	}
	q := a / b
	r := a % b
	if r == 0 {
		return q, nil
	}
	rb, bb := r, b
	if rb < 0 {
		rb = -rb
	}
	if bb < 0 {
		bb = -bb
	}
	if rb*2 >= bb {
		if a >= 0 {
			q++
		} else {
			q--
		}
	}
	return q, nil
}

// MulDivChecked returns (a*b)/c rounded half away from zero, checking overflow
// and division by zero. It is the core "area times rate" primitive.
func MulDivChecked(a, b, c int64) (int64, error) {
	if c == 0 {
		return 0, ErrDivZero
	}
	p, err := MulChecked(a, b)
	if err != nil {
		return 0, err
	}
	return DivRoundChecked(p, c)
}

// NonNegative returns v or ErrNegative, used to reject negative lengths,
// masses, scales and time intervals before any arithmetic proceeds.
func NonNegative(v int64) (int64, error) {
	if v < 0 {
		return 0, ErrNegative
	}
	return v, nil
}
