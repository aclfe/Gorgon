package for_condition_true

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

func TestInfiniteLoop(t *testing.T) {
	t.Parallel()
	if got := InfiniteLoop(); got != 101 {
		t.Errorf("InfiniteLoop() = %d, want 101", got)
	}
}