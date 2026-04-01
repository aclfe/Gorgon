package inc_dec_flip

import "testing"

func TestSimpleInc(t *testing.T) {
	t.Parallel()
	if got := SimpleInc(); got != 1 {
		t.Errorf("SimpleInc() = %d, want 1", got)
	}
}

func TestSimpleDec(t *testing.T) {
	t.Parallel()
	if got := SimpleDec(); got != 4 {
		t.Errorf("SimpleDec() = %d, want 4", got)
	}
}
