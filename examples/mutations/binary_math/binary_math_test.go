package binary_math

import "testing"

func TestMod(t *testing.T) {
	t.Parallel()
	if got := Mod(10, 3); got != 1 {
		t.Errorf("Mod(10, 3) = %d, want 1", got)
	}
}

func TestBitAnd(t *testing.T) {
	t.Parallel()
	if got := BitAnd(6, 3); got != 2 {
		t.Errorf("BitAnd(6, 3) = %d, want 2", got)
	}
}

func TestBitOr(t *testing.T) {
	t.Parallel()
	if got := BitOr(6, 3); got != 7 {
		t.Errorf("BitOr(6, 3) = %d, want 7", got)
	}
}

func TestShiftLeft(t *testing.T) {
	t.Parallel()
	if got := ShiftLeft(2); got != 8 {
		t.Errorf("ShiftLeft(2) = %d, want 8", got)
	}
}

func TestShiftRight(t *testing.T) {
	t.Parallel()
	if got := ShiftRight(8); got != 2 {
		t.Errorf("ShiftRight(8) = %d, want 2", got)
	}
}
