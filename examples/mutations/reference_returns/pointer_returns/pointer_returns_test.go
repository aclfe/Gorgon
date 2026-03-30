package pointer_returns

import "testing"

func TestGetPointer(t *testing.T) {
	t.Parallel()
	if got := GetPointer(); got == nil || *got != 42 {
		t.Errorf("GetPointer() = %v, want non-nil pointer to 42", got)
	}
}

func TestGetStringPointer(t *testing.T) {
	t.Parallel()
	if got := GetStringPointer(); got == nil || *got != "hello" {
		t.Errorf("GetStringPointer() = %v, want non-nil pointer to \"hello\"", got)
	}
}

func TestGetNilPointer(t *testing.T) {
	t.Parallel()
	if got := GetNilPointer(); got != nil {
		t.Errorf("GetNilPointer() = %v, want nil", got)
	}
}
