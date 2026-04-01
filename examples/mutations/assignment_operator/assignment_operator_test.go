package assignment_operator

import "testing"

func TestAddToCounter(t *testing.T) {
	counter := 10
	AddToCounter(&counter, 5)
	if counter != 15 {
		t.Errorf("expected counter to be 15, got %d", counter)
	}
}

func TestDouble(t *testing.T) {
	result := Double(5)
	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}
}

func TestTriple(t *testing.T) {
	result := Triple(5)
	if result != 15 {
		t.Errorf("expected 15, got %d", result)
	}
}

func TestHalve(t *testing.T) {
	result := Halve(10)
	if result != 5 {
		t.Errorf("expected 5, got %d", result)
	}
	result = Halve(0)
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}
