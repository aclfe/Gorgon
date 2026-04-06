package nil_check_removal

import "testing"

func TestRunService(t *testing.T) {
	t.Parallel()
	svc := &Service{Name: "test"}
	result := RunService(svc)
	if result != "test started" {
		t.Errorf("RunService() = %q, want %q", result, "test started")
	}
}

func TestRunServiceNil(t *testing.T) {
	t.Parallel()
	result := RunService(nil)
	if result != "no service" {
		t.Errorf("RunService(nil) = %q, want %q", result, "no service")
	}
}

func TestCleanupService(t *testing.T) {
	t.Parallel()
	svc := &Service{Name: "test"}
	CleanupService(svc)
	if svc.Name != "stopped" {
		t.Errorf("CleanupService() Name = %q, want %q", svc.Name, "stopped")
	}
}

func TestCleanupServiceNil(t *testing.T) {
	t.Parallel()
	CleanupService(nil)
}

func TestGetName(t *testing.T) {
	t.Parallel()
	svc := &Service{Name: "test"}
	if got := GetName(svc); got != "test" {
		t.Errorf("GetName() = %q, want %q", got, "test")
	}
}

func TestGetNameNil(t *testing.T) {
	t.Parallel()
	if got := GetName(nil); got != "unknown" {
		t.Errorf("GetName(nil) = %q, want %q", got, "unknown")
	}
}

func TestProcessItems(t *testing.T) {
	t.Parallel()
	items := []*Item{{Value: 10}, {Value: 20}}
	if got := ProcessItems(items); got != 30 {
		t.Errorf("ProcessItems() = %d, want 30", got)
	}
}

func TestProcessItemsWithNil(t *testing.T) {
	t.Parallel()
	items := []*Item{{Value: 10}, nil, {Value: 20}}
	if got := ProcessItems(items); got != 30 {
		t.Errorf("ProcessItems() = %d, want 30", got)
	}
}
