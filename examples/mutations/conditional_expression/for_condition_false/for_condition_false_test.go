package for_condition_false

import "testing"

func TestSumUntilLimit(t *testing.T) {
	t.Parallel()
	if got := SumUntilLimit(0, 5); got != 10 {
		t.Errorf("SumUntilLimit(0, 5) = %d, want 10", got)
	}
}

func TestLoopWithCondition(t *testing.T) {
	t.Parallel()
	if got := LoopWithCondition(3); got != 6 {
		t.Errorf("LoopWithCondition(3) = %d, want 6", got)
	}
}