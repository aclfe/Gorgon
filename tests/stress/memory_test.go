//go:build stress
// +build stress

package stress

import (
	"runtime"
	"testing"
)

// TestStress_MemoryUsage verifies reasonable memory usage
func TestStress_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	dir := createLargeProject(t, 100)

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	_, err := runGorgon(t, dir)
	if err != nil {
		t.Fatalf("Gorgon failed: %v", err)
	}

	runtime.ReadMemStats(&m2)

	allocMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memory allocated: %.2f MB", allocMB)

	// Verify memory usage is reasonable (adjust threshold as needed)
	if allocMB > 1000 {
		t.Errorf("Memory usage too high: %.2f MB", allocMB)
	}
}

// TestStress_NoMemoryLeaks verifies no memory leaks
func TestStress_NoMemoryLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	t.Skip("TODO: Run multiple times and verify memory doesn't grow")
}

// TestStress_NoGoroutineLeaks verifies no goroutine leaks
func TestStress_NoGoroutineLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	dir := createLargeProject(t, 10)

	before := runtime.NumGoroutine()

	_, err := runGorgon(t, dir)
	if err != nil {
		t.Fatalf("Gorgon failed: %v", err)
	}

	runtime.GC()
	after := runtime.NumGoroutine()

	// Allow some tolerance for background goroutines
	if after > before+10 {
		t.Errorf("Goroutine leak detected: before=%d, after=%d", before, after)
	}

	t.Logf("Goroutines: before=%d, after=%d", before, after)
}

// TestStress_NoFileDescriptorLeaks verifies no FD leaks
func TestStress_NoFileDescriptorLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	t.Skip("TODO: Monitor file descriptors and verify no leaks")
}
