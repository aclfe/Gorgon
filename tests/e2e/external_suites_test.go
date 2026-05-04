//go:build e2e
// +build e2e

package e2e

import (
	"path/filepath"
	"testing"
)

// TestExternalSuites_KillMutations verifies external suites actually kill mutations
func TestExternalSuites_KillMutations(t *testing.T) {
	// TODO: Rebuild from scratch
	t.Skip("TODO: Rebuild from scratch")
}

// TestExternalSuites_TagsIntegration verifies build tags filter external tests
func TestExternalSuites_TagsIntegration(t *testing.T) {
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

// TestExternalSuites_BothEnabled verifies unit and external tests run together
func TestExternalSuites_BothEnabled(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_BothEnabled/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/core")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Both enabled should have results from both test types
	stats := debugKillStats(t, report, "BothEnabled", true, true)
	expectInternalKilled(t, stats, "BothEnabled")
	expectExternalKilled(t, stats, "BothEnabled")

	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
}

// TestExternalSuites_UnitOnly verifies unit tests alone work
func TestExternalSuites_UnitOnly(t *testing.T) {
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
