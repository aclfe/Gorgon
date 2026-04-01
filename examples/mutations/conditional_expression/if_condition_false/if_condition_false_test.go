package if_condition_false

import "testing"

func TestMax(t *testing.T) {
	t.Parallel()
	if got := Max(5, 3); got != 5 {
		t.Errorf("Max(5, 3) = %d, want 5", got)
	}
}

func TestSign(t *testing.T) {
	t.Parallel()
	if got := Sign(5); got != 1 {
		t.Errorf("Sign(5) = %d, want 1", got)
	}
	if got := Sign(-1); got != 0 {
		t.Errorf("Sign(-1) = %d, want 0", got)
	}
}