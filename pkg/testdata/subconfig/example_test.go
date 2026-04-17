package testdata

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Errorf("Add(2, 3) = %d, want 5", Add(2, 3))
	}
}

func TestSubtract(t *testing.T) {
	if Subtract(5, 3) != 2 {
		t.Errorf("Subtract(5, 3) = %d, want 2", Subtract(5, 3))
	}
}
