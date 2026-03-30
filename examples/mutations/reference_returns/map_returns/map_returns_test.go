package map_returns

import "testing"

func TestGetMap(t *testing.T) {
	t.Parallel()
	got := GetMap()
	if got == nil || got["a"] != 1 || got["b"] != 2 {
		t.Errorf("GetMap() = %v, want map[string]int{\"a\": 1, \"b\": 2}", got)
	}
}

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
