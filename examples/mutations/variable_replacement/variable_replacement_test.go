package variable_replacement

import "testing"

func TestCalculate(t *testing.T) {
	t.Parallel()
	if got := Calculate(3, 2); got != 5 {
		t.Errorf("Calculate(3, 2) = %d, want 5", got)
	}
}

func TestProcessValues(t *testing.T) {
	t.Parallel()
	if got := ProcessValues(10, 5); got != 15 {
		t.Errorf("ProcessValues(10, 5) = %d, want 15", got)
	}
}

func TestFindMax(t *testing.T) {
	t.Parallel()
	if got := FindMax(5, 10); got != 10 {
		t.Errorf("FindMax(5, 10) = %d, want 10", got)
	}
	if got := FindMax(10, 5); got != 10 {
		t.Errorf("FindMax(10, 5) = %d, want 10", got)
	}
}
