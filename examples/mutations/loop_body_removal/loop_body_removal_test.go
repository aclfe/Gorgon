package loop_body_removal

import "testing"

func TestClassicFor(t *testing.T) {
	t.Parallel()
	sum := 0
	ClassicFor(&sum)
	if sum != 45 {
		t.Errorf("ClassicFor sum = %d, want 45", sum)
	}
}

func TestWhileStyle(t *testing.T) {
	t.Parallel()
	sum := 0
	WhileStyle(&sum)
	if sum != 45 {
		t.Errorf("WhileStyle sum = %d, want 45", sum)
	}
}

func TestRangeLoop(t *testing.T) {
	t.Parallel()
	result := 0
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	RangeLoop(&result, m)
	if result != 6 {
		t.Errorf("RangeLoop result = %d, want 6", result)
	}
}

func TestInfiniteLoop(t *testing.T) {
	t.Parallel()
	done := false
	InfiniteLoop(&done)
	if !done {
		t.Error("InfiniteLoop expected done to be true")
	}
}
