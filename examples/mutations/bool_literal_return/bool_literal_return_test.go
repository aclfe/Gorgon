package bool_literal_return

import "testing"

func TestIsPositive(t *testing.T) {
	t.Parallel()
	if !IsPositive(5) {
		t.Error("IsPositive(5) expected true")
	}
	if IsPositive(-1) {
		t.Error("IsPositive(-1) expected false")
	}
	if IsPositive(0) {
		t.Error("IsPositive(0) expected false")
	}
}

func TestHasPrefix(t *testing.T) {
	t.Parallel()
	if !HasPrefix("hello", "hel") {
		t.Error("HasPrefix(hello, hel) expected true")
	}
	if HasPrefix("hello", "world") {
		t.Error("HasPrefix(hello, world) expected false")
	}
}

func TestIsEqual(t *testing.T) {
	t.Parallel()
	if !IsEqual(5, 5) {
		t.Error("IsEqual(5, 5) expected true")
	}
	if IsEqual(5, 3) {
		t.Error("IsEqual(5, 3) expected false")
	}
}

func TestIsEmpty(t *testing.T) {
	t.Parallel()
	if !IsEmpty("") {
		t.Error("IsEmpty('') expected true")
	}
	if IsEmpty("hello") {
		t.Error("IsEmpty('hello') expected false")
	}
}
