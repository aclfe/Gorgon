package constant_replacement

import "testing"

func TestGetMaxRetries(t *testing.T) {
	t.Parallel()
	if got := GetMaxRetries(); got != 3 {
		t.Errorf("GetMaxRetries() = %d, want 3", got)
	}
}

func TestGetDefaultPort(t *testing.T) {
	t.Parallel()
	if got := GetDefaultPort(); got != 8080 {
		t.Errorf("GetDefaultPort() = %d, want 8080", got)
	}
}

func TestGetEmptyString(t *testing.T) {
	t.Parallel()
	if got := GetEmptyString(); got != "" {
		t.Errorf("GetEmptyString() = %q, want \"\"", got)
	}
}

func TestGetDefaultScale(t *testing.T) {
	t.Parallel()
	if got := GetDefaultScale(); got != 1.5 {
		t.Errorf("GetDefaultScale() = %f, want 1.5", got)
	}
}

func TestGetMarker(t *testing.T) {
	t.Parallel()
	if got := GetMarker(); got != 'x' {
		t.Errorf("GetMarker() = %c, want 'x'", got)
	}
}

func TestGetMaxConnections(t *testing.T) {
	t.Parallel()
	if got := GetMaxConnections(); got != 100 {
		t.Errorf("GetMaxConnections() = %d, want 100", got)
	}
}
