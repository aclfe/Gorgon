package logical_operator

import "testing"

func TestIsAdult(t *testing.T) {
	if !IsAdult(20, true) {
		t.Error("expected 20 year old with license to be adult")
	}
	if IsAdult(16, true) {
		t.Error("expected 16 year old to not be adult")
	}
	if IsAdult(20, false) {
		t.Error("expected adult without license to not be valid adult")
	}
}

func TestCanVote(t *testing.T) {
	if !CanVote(25, true) {
		t.Error("expected 25 year old citizen to vote")
	}
	if CanVote(16, true) {
		t.Error("expected minor to not vote")
	}
	if CanVote(25, false) {
		t.Error("expected non-citizen to not vote")
	}
}

func TestHasDiscount(t *testing.T) {
	if !HasDiscount(true, false) {
		t.Error("expected member to have discount")
	}
	if !HasDiscount(false, true) {
		t.Error("expected order above 100 to have discount")
	}
	if HasDiscount(false, false) {
		t.Error("expected no discount for non-member and small order")
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid(true, false) {
		t.Error("expected verified email to be valid")
	}
	if !IsValid(false, true) {
		t.Error("expected valid payment to be valid")
	}
	if IsValid(false, false) {
		t.Error("expected no verification to be invalid")
	}
}
