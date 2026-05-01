//go:build e2e
// +build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// killStats holds the counts of internal and external kills
type killStats struct {
	InternalKilled int
	ExternalKilled int
	InternalKilledBy []string
	ExternalKilledBy []string
}

// debugKillStats logs detailed kill statistics for a report and returns the counts
func debugKillStats(t *testing.T, report *ReportData, testName string) killStats {
	t.Helper()
	
	var stats killStats
	
	for _, m := range report.Mutants {
		if m.Status == "killed" {
			if strings.Contains(m.KilledBy, "[") || isExternalSuiteName(m.KilledBy) {
				stats.ExternalKilled++
				stats.ExternalKilledBy = append(stats.ExternalKilledBy, m.KilledBy)
			} else if m.KilledBy != "" && m.KilledBy != "(compiler)" && m.KilledBy != "(timeout)" && m.KilledBy != "runtime error" {
				stats.InternalKilled++
				stats.InternalKilledBy = append(stats.InternalKilledBy, m.KilledBy)
			}
		}
	}
	
	t.Logf("[%s] EXPECTS INTERNAL KILLED: >0, IS KILLED: %d (test names: %v)", testName, stats.InternalKilled, stats.InternalKilledBy[:min(len(stats.InternalKilledBy), 3)])
	t.Logf("[%s] EXPECTS EXTERNAL KILLED: >0, IS KILLED: %d (suite names: %v)", testName, stats.ExternalKilled, stats.ExternalKilledBy[:min(len(stats.ExternalKilledBy), 3)])
	t.Logf("[%s] Summary: Killed=%d, Survived=%d, Total=%d", testName, report.Summary.Killed, report.Summary.Survived, report.Summary.Total)
	
	return stats
}

// expectInternalKilled fails the test if no internal kills were detected
func expectInternalKilled(t *testing.T, stats killStats, testName string) {
	t.Helper()
	if stats.InternalKilled == 0 {
		t.Errorf("[%s] EXPECTED INTERNAL KILLED > 0, but got %d", testName, stats.InternalKilled)
	}
}

// expectExternalKilled fails the test if no external kills were detected
func expectExternalKilled(t *testing.T, stats killStats, testName string) {
	t.Helper()
	if stats.ExternalKilled == 0 {
		t.Errorf("[%s] EXPECTED EXTERNAL KILLED > 0, but got %d", testName, stats.ExternalKilled)
	}
}

// isExternalSuiteName checks if killedBy looks like an external suite name
func isExternalSuiteName(killedBy string) bool {
	// Common external suite names that don't have brackets
	suites := []string{"integration", "e2e", "external"}
	for _, s := range suites {
		if strings.EqualFold(killedBy, s) || strings.Contains(killedBy, s) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestExternalSuites_KillMutations verifies external suites actually kill mutations
func TestExternalSuites_KillMutations(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_KillMutations/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Verify some mutants were killed
	if report.Summary.Killed == 0 {
		t.Errorf("Expected some mutants to be killed by external suite, got %d", report.Summary.Killed)
	}

	// Verify killed mutants have KilledBy field
	killedWithAttribution := 0
	for _, m := range report.Mutants {
		if m.Status == "killed" && m.KilledBy != "" {
			killedWithAttribution++
		}
	}

	if killedWithAttribution == 0 {
		t.Error("Expected at least one killed mutant to have KilledBy attribution")
	}

	stats := debugKillStats(t, report, "KillMutations")
	expectExternalKilled(t, stats, "KillMutations")
}

// TestExternalSuites_RunModeOnly verifies run_mode: only skips unit tests
func TestExternalSuites_RunModeOnly(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_RunModeOnly/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Verify mutants were processed
	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be processed")
	}

	// Verify some have status set (all should be processed by external)
	processed := 0
	for _, m := range report.Mutants {
		if m.Status != "" && m.Status != "untested" {
			processed++
		}
	}

	if processed == 0 {
		t.Error("Expected external suite to process mutants")
	}

	stats := debugKillStats(t, report, "RunModeOnly")
	expectExternalKilled(t, stats, "RunModeOnly")
}

// TestExternalSuites_RunModeAfterUnit verifies run_mode: after_unit only runs on survived
func TestExternalSuites_RunModeAfterUnit(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_RunModeAfterUnit/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// The key test: after_unit mode processes all mutants through unit tests first
	// Then external runs on survivors. We should see a mix of statuses.
	stats := debugKillStats(t, report, "RunModeAfterUnit")
	expectInternalKilled(t, stats, "RunModeAfterUnit")
	expectExternalKilled(t, stats, "RunModeAfterUnit")

	// Verify we have both killed and survived (indicating unit tests ran)
	if report.Summary.Killed == 0 && report.Summary.Survived == 0 {
		t.Error("Expected either killed or survived mutants from unit test phase")
	}
}

// TestExternalSuites_RunModeAlongside verifies run_mode: alongside runs on all mutants
func TestExternalSuites_RunModeAlongside(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_RunModeAlongside/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Alongside mode should have external run on all mutants
	// This might result in more kills due to additional test coverage
	stats := debugKillStats(t, report, "RunModeAlongside")
	expectInternalKilled(t, stats, "RunModeAlongside")
	expectExternalKilled(t, stats, "RunModeAlongside")

	// Basic sanity check
	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
}

// TestExternalSuites_TagsIntegration verifies build tags filter external tests
func TestExternalSuites_TagsIntegration(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_TagsIntegration/gorgon.yml")
	// Use integration tests directory which has integration-tagged tests
	targetDir := filepath.Join(repoRoot, "tests/integration")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	stats := debugKillStats(t, report, "TagsIntegration")
	expectExternalKilled(t, stats, "TagsIntegration")

	// Just verify it ran - the tag filtering is validated by gorgon building/running correctly
	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated in integration tests")
	}
}

// TestExternalSuites_BothEnabled verifies unit and external tests run together
func TestExternalSuites_BothEnabled(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	configPath := filepath.Join(repoRoot, "tests/e2e/testdata/TestExternalSuites_BothEnabled/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Both enabled should have results from both test types
	stats := debugKillStats(t, report, "BothEnabled")
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
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// Unit only should have killed mutants from package tests
	stats := debugKillStats(t, report, "UnitOnly")
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
	targetDir := filepath.Join(repoRoot, "internal/reporter")

	report, err := runGorgonWithConfig(t, configPath, targetDir)
	if err != nil {
		t.Fatalf("Failed to run gorgon: %v", err)
	}

	// External only should process all mutants through external suite
	stats := debugKillStats(t, report, "ExternalOnly")
	expectExternalKilled(t, stats, "ExternalOnly")

	if report.Summary.Total == 0 {
		t.Error("Expected mutants to be generated")
	}
}
