package negate_condition

import "testing"

func TestGetStatus(t *testing.T) {
	if GetStatus(60) != "pass" {
		t.Error("score 60 should pass")
	}
	if GetStatus(59) != "fail" {
		t.Error("score 59 should fail")
	}
	if GetStatus(100) != "pass" {
		t.Error("score 100 should pass")
	}
	if GetStatus(0) != "fail" {
		t.Error("score 0 should fail")
	}
}

func TestCheckAccess(t *testing.T) {
	if !CheckAccess(5, 3) {
		t.Error("level 5 >= required 3 should allow access")
	}
	if !CheckAccess(3, 3) {
		t.Error("level 3 >= required 3 should allow access")
	}
	if CheckAccess(2, 3) {
		t.Error("level 2 < required 3 should deny access")
	}
}
