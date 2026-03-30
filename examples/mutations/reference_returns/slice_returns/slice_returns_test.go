package slice_returns

import "testing"

func TestGetStringSlice(t *testing.T) {
	t.Parallel()
	got := GetStringSlice()
	if got == nil || len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("GetStringSlice() = %v, want []string{\"a\", \"b\", \"c\"}", got)
	}
}

func TestGetEmptySlice(t *testing.T) {
	t.Parallel()
	got := GetEmptySlice()
	if got == nil || len(got) != 0 {
		t.Errorf("GetEmptySlice() = %v, want empty slice", got)
	}
}

func TestGetNilSlice(t *testing.T) {
	t.Parallel()
	if got := GetNilSlice(); got != nil {
		t.Errorf("GetNilSlice() = %v, want nil", got)
	}
}
