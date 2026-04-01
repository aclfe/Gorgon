package boundary_value

import "testing"

func TestInRange(t *testing.T) {
	if !InRange(5, 0, 10) {
		t.Error("expected 5 to be in range 0-10")
	}
	if InRange(0, 0, 10) {
		t.Error("expected 0 to be out of range 0-10")
	}
}

func TestIsBelow(t *testing.T) {
	if !IsBelow(3, 5) {
		t.Error("expected 3 to be below 5")
	}
	if IsBelow(5, 5) {
		t.Error("expected 5 to not be below 5")
	}
}

func TestIsAbove(t *testing.T) {
	if !IsAbove(7, 5) {
		t.Error("expected 7 to be above 5")
	}
	if IsAbove(5, 5) {
		t.Error("expected 5 to not be above 5")
	}
}

func TestAtLeast(t *testing.T) {
	if !AtLeast(5, 5) {
		t.Error("expected 5 to be at least 5")
	}
	if AtLeast(6, 5) {
		t.Error("expected 6 to not be at least 5")
	}
}

func TestAtMost(t *testing.T) {
	if !AtMost(5, 5) {
		t.Error("expected 5 to be at most 5")
	}
	if AtMost(4, 5) {
		t.Error("expected 4 to not be at most 5")
	}
}
