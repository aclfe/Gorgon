package panic_removal

import (
	"testing"
)

func TestRequirePositive(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-positive value")
		}
	}()
	RequirePositive(-1)
}

func TestRequireNotZero(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for zero value")
		}
	}()
	RequireNotZero(0)
}

func TestGetFromMap(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	if got := GetFromMap(m, "a"); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestGetItem(t *testing.T) {
	items := []string{"x", "y", "z"}
	if got := GetItem(items, 1); got != "y" {
		t.Errorf("expected 'y', got %q", got)
	}
}

func TestMustNonEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty string")
		}
	}()
	MustNonEmpty("")
}
