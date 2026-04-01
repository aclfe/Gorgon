package typeedgecases

import "testing"

func TestArrayWithConst(t *testing.T) {
	t.Parallel()
	if got := ArrayWithConst(); len(got) != BlockSize {
		t.Errorf("expected %d bytes, got %d", BlockSize, len(got))
	}
}

func TestPointerToArray(t *testing.T) {
	t.Parallel()
	if got := PointerToArray(); got == nil {
		t.Error("expected non-nil pointer")
	}
}

func TestSliceOfArrays(t *testing.T) {
	t.Parallel()
	if got := SliceOfArrays(); len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

func TestMapWithArrayKey(t *testing.T) {
	t.Parallel()
	if got := MapWithArrayKey(); len(got) != 0 {
		t.Errorf("expected empty map, got %d", len(got))
	}
}

func TestChanOfArrays(t *testing.T) {
	t.Parallel()
	if got := ChanOfArrays(); got == nil {
		t.Error("expected non-nil channel")
	}
}

func TestNestedPointers(t *testing.T) {
	t.Parallel()
	if got := NestedPointers(); got != nil {
		t.Error("expected nil")
	}
}

func TestEllipsisParam(t *testing.T) {
	t.Parallel()
	EllipsisParam("a", "b")
}

func TestInterfaceReturn(t *testing.T) {
	t.Parallel()
	if got := InterfaceReturn(); got != nil {
		t.Error("expected nil")
	}
}

func TestArrayWithBinary(t *testing.T) {
	t.Parallel()
	if got := ArrayWithBinary(); len(got) != 2 {
		t.Errorf("expected 2 bytes, got %d", len(got))
	}
}

func TestArrayWithParen(t *testing.T) {
	t.Parallel()
	if got := ArrayWithParen(); len(got) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(got))
	}
}
