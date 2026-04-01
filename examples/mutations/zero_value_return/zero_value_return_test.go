package zero_value_return

import "testing"

func TestGetNumber(t *testing.T) {
	t.Parallel()
	if got := GetNumber(); got != 42 {
		t.Errorf("GetNumber() = %d, want 42", got)
	}
}

func TestGetString(t *testing.T) {
	t.Parallel()
	if got := GetString(); got != "hello" {
		t.Errorf("GetString() = %q, want \"hello\"", got)
	}
}

func TestGetSlice(t *testing.T) {
	t.Parallel()
	if got := GetSlice(); got == nil || len(got) != 3 {
		t.Errorf("GetSlice() = %v, want []int{1, 2, 3}", got)
	}
}

func TestGetMap(t *testing.T) {
	t.Parallel()
	if got := GetMap(); got == nil || got["a"] != 1 {
		t.Errorf("GetMap() = %v, want map[string]int{a: 1}", got)
	}
}

func TestGetError(t *testing.T) {
	t.Parallel()
	if got := GetError(); got == nil {
		t.Errorf("GetError() = nil, want error")
	}
}

func TestGetNil(t *testing.T) {
	t.Parallel()
	if got := GetNil(); got != nil {
		t.Errorf("GetNil() = %v, want nil", got)
	}
}
