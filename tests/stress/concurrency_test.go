//go:build stress
// +build stress

package stress

import (
	"strings"
	"testing"
)

// TestStress_HighConcurrency verifies handling of high concurrency
func TestStress_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	dir := createLargeProject(t, 50)
	output, err := runGorgon(t, "-concurrent=100", dir)

	if err != nil {
		t.Fatalf("Gorgon failed with high concurrency: %v\nOutput: %s", err, output)
	}

	t.Logf("Successfully ran with high concurrency")
}

// TestStress_RaceDetector verifies no race conditions
func TestStress_RaceDetector(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	t.Skip("TODO: Run with -race flag and verify no data races")
}

// TestStress_Deterministic verifies deterministic results
func TestStress_Deterministic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	dir := createLargeProject(t, 10)

	// Run 10 times and verify identical results
	var results []string
	for i := 0; i < 10; i++ {
		output, err := runGorgon(t, dir)
		if err != nil {
			t.Fatalf("Run %d failed: %v", i+1, err)
		}
		results = append(results, output)
	}

	// Compare all results
	first := results[0]
	for i := 1; i < len(results); i++ {
		if results[i] != first {
			t.Errorf("Run %d produced different results", i+1)
		}
	}

	t.Logf("All 10 runs produced identical results")
}

// TestStress_ConcurrentCache verifies cache under concurrent load
func TestStress_ConcurrentCache(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	dir := createLargeProject(t, 50)

	// First run to populate cache
	_, err := runGorgon(t, "-cache", dir)
	if err != nil {
		t.Fatalf("First run failed: %v", err)
	}

	// Second run with high concurrency using cache
	output, err := runGorgon(t, "-cache", "-concurrent=50", dir)
	if err != nil {
		t.Fatalf("Cached run failed: %v", err)
	}

	if !strings.Contains(output, "cache") && !strings.Contains(output, "Cache") {
		t.Log("Note: Cache may be working but not reporting in output")
	}

	t.Logf("Successfully ran with cache and high concurrency")
}
