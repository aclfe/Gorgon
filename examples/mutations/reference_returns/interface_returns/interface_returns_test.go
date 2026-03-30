package interface_returns

import "testing"

func TestGetInterface(t *testing.T) {
	t.Parallel()
	if got := GetInterface(); got != "hello" {
		t.Errorf("GetInterface() = %v, want \"hello\"", got)
	}
}

func TestGetIntInterface(t *testing.T) {
	t.Parallel()
	if got := GetIntInterface(); got != 42 {
		t.Errorf("GetIntInterface() = %v, want 42", got)
	}
}
