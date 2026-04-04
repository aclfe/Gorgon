package negate_condition

import "testing"

func TestGetStatus(t *testing.T) {
	if got := GetStatus(80); got != "pass" {
		t.Errorf("expected pass, got %s", got)
	}
	if got := GetStatus(40); got != "fail" {
		t.Errorf("expected fail, got %s", got)
	}
}

func TestCheckAccess(t *testing.T) {
	if !CheckAccess(5, 3) {
		t.Error("expected level 5 to meet requirement 3")
	}
	if CheckAccess(2, 5) {
		t.Error("expected level 2 to not meet requirement 5")
	}
}
