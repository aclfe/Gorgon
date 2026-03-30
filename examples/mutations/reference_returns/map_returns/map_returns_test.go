package map_returns

import "testing"

func TestGetStringMap(t *testing.T) {
	t.Parallel()
	got := GetStringMap()
	if got == nil || got["key"] != "value" {
		t.Errorf("GetStringMap() = %v, want map[string]string{\"key\": \"value\"}", got)
	}
}

func TestGetEmptyMap(t *testing.T) {
	t.Parallel()
	got := GetEmptyMap()
	if got == nil || len(got) != 0 {
		t.Errorf("GetEmptyMap() = %v, want empty map", got)
	}
}

func TestGetNilMap(t *testing.T) {
	t.Parallel()
	if got := GetNilMap(); got != nil {
		t.Errorf("GetNilMap() = %v, want nil", got)
	}
}
