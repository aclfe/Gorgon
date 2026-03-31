package negate_condition

import "testing"

func TestIsValid(t *testing.T) {
	t.Parallel()
	if !IsValid(20) {
		t.Error("expected 20 to be valid")
	}
	if IsValid(16) {
		t.Error("expected 16 to be invalid")
	}
}

func TestHasAccess(t *testing.T) {
	t.Parallel()
	if !HasAccess(true, false) {
		t.Error("expected admin to have access")
	}
	if !HasAccess(false, true) {
		t.Error("expected permission to have access")
	}
	if HasAccess(false, false) {
		t.Error("expected no access without admin or permission")
	}
}

func TestCheckBalance(t *testing.T) {
	t.Parallel()
	if !CheckBalance(50, 100) {
		t.Error("expected 50 to be within limit")
	}
	if CheckBalance(150, 100) {
		t.Error("expected 150 to exceed limit")
	}
}

func TestAlreadyNegated(t *testing.T) {
	t.Parallel()
	if alreadyNegated(true) {
		t.Error("expected !true to be false")
	}
}
