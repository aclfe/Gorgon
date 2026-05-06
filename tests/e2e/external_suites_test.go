//go:build e2e
// +build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExternalSuites_KillMutations verifies external suites actually kill mutations
func TestExternalSuites_KillMutations(t *testing.T) {
	// TODO: Rebuild from scratch
	t.Skip("TODO: Rebuild from scratch")
}

// TestExternalSuites_TagsIntegration verifies build tags filter external tests
func TestExternalSuites_TagsIntegration(t *testing.T) {
	t.Parallel()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_TagsIntegration/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/core")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	stats := debugKillStats(t, report, "TagsIntegration", false, true)
	expectExternalKilled(t, stats, "TagsIntegration")

	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
}

// TestExternalSuites_BothEnabled verifies unit and external tests run together.
// Default run_mode (after-unit): unit tests kill what they can, external suites
// then run against the survivors. The test asserts both phases EXECUTED — kills
// from each phase are not required because either suite may legitimately cover
// everything the other would have caught.
func TestExternalSuites_BothEnabled(t *testing.T) {
	t.Parallel()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_BothEnabled/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/core")

	stdout, report, err := runGorgonWithConfigCapture(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	stats := debugKillStats(t, report, "BothEnabled", true, true)
	expectInternalKilled(t, stats, "BothEnabled")
	expectExternalKilled(t, stats, "BothEnabled")

	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
	if report.Summary.Killed == 0 {
		t.Errorf("[BothEnabled] expected total killed > 0 across both phases, got 0")
	}

	// Verify both phases actually executed by scanning gorgon's log output.
	// "[UNIT] Running" fires from compileAndRunPackages; "[EXTERNAL] Running"
	// fires from runExternalPhaseWithBinaries.
	if !strings.Contains(stdout, "[UNIT] Running") {
		t.Errorf("[BothEnabled] unit phase did not execute (no '[UNIT] Running' marker in gorgon output)")
	}
	if !strings.Contains(stdout, "[EXTERNAL] Running") {
		t.Errorf("[BothEnabled] external phase did not execute (no '[EXTERNAL] Running' marker in gorgon output)")
	}

	// Sanity: at least one kill must come from somewhere — i.e. the run is not
	// catastrophically broken. We do not require both phases to contribute
	// kills (the package they target may be fully covered by one suite).
	if stats.InternalKilled == 0 && stats.ExternalKilled == 0 {
		t.Errorf("[BothEnabled] no kills attributable to either phase — at least one is expected when both are enabled")
	}
}

// TestExternalSuites_UnitOnly verifies unit tests alone work
func TestExternalSuites_UnitOnly(t *testing.T) {
	t.Parallel()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_UnitOnly/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/core")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Unit only should have killed mutants from package tests
	stats := debugKillStats(t, report, "UnitOnly", true, false)
	expectInternalKilled(t, stats, "UnitOnly")

	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
}

// TestExternalSuites_ExternalOnly verifies external tests alone work
func TestExternalSuites_ExternalOnly(t *testing.T) {
	t.Parallel()
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_ExternalOnly/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/core")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// External only should process all mutants through external suite
	stats := debugKillStats(t, report, "ExternalOnly", false, true)
	expectExternalKilled(t, stats, "ExternalOnly")

	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
}

// allow tests to run without tags? 
// allow multiple external tests tags?
// internal doesn't work after "before_unit" phasse in external. 
// allow a singular file in integration / internal to kill tests?
// Killed=4, Survived=3930, Total=4168. Why all survived? they arne't tested. survied is only when test isn't killed, when tested. It should have been 3930  untested