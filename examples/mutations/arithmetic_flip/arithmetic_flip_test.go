package arithmetic_flip

import "testing"

func TestWeird1(t *testing.T) {
	t.Parallel()
	if got := Add(1, 2); got != 3 {
		t.Errorf("Add(1, 2) = %d, want 3", got)
	}
}

func TestWeird2(t *testing.T) {
	t.Parallel()
	if got := Subtract(5, 3); got != 2 {
		t.Errorf("Subtract(5, 3) = %d, want 2", got)
	}
}

func TestWeird3(t *testing.T) {
	t.Parallel()
	if got := Multiply(4, 6); got != 24 {
		t.Errorf("Multiply(4, 6) = %d, want 24", got)
	}
}
