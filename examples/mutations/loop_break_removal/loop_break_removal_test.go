package loop_break_removal

import "testing"

func TestClassicBreak(t *testing.T) {
	t.Parallel()
	result := 0
	ClassicBreak(&result)
	if result != 10 {
		t.Errorf("ClassicBreak result = %d, want 10", result)
	}
}

func TestRangeBreak(t *testing.T) {
	t.Parallel()
	result := 0
	items := []int{1, 2, 3, -1, 4, 5}
	RangeBreak(&result, items)
	if result != 6 {
		t.Errorf("RangeBreak result = %d, want 6", result)
	}
}

func TestNestedBreak(t *testing.T) {
	t.Parallel()
	result := 0
	NestedBreak(&result)
	if result != 3 {
		t.Errorf("NestedBreak result = %d, want 3", result)
	}
}

func TestMultipleBreaks(t *testing.T) {
	t.Parallel()
	found := false
	MultipleBreaks(&found)
	if !found {
		t.Error("MultipleBreaks expected found to be true")
	}
}
