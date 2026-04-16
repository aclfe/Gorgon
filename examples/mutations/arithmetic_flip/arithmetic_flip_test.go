package arithmetic_flip

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add failed")
	}
}

func TestSubtract(t *testing.T) {
	if Subtract(5, 3) != 2 {
		t.Error("Subtract failed")
	}
}

func TestMultiply(t *testing.T) {
	if Multiply(3, 4) != 12 {
		t.Error("Multiply failed")
	}
}

func TestDivide(t *testing.T) {
	if Divide(12, 3) != 4 {
		t.Error("Divide failed")
	}
}

func TestAddCompound(t *testing.T) {
	if AddCompound(1, 2, 3) != 6 {
		t.Error("AddCompound failed")
	}
}
