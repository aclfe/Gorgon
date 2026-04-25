//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Stronger assertions? Not sure frankly. 

// TestExternalSuites_BothEnabled verifies that both unit and external tests run when both are enabled
func TestExternalSuites_BothEnabled(t *testing.T) {
	configContent := `unit_tests_enabled: true
external_suites:
  enabled: true
  run_mode: after_unit
  suites:
    - name: all-tests
      paths: [./tests/...]
`
	output := runGorgonWithConfig(t, configContent, "./examples/mutations/arithmetic_flip")

	if !strings.Contains(output, "[EXTERNAL] Running external suite phase") {
		t.Error("Expected external suite phase to run")
	}

	if !strings.Contains(output, "Built") {
		t.Error("Expected external suite binaries to be built")
	}

	if !strings.Contains(output, "[all-tests]") {
		t.Error("Expected external suite to kill mutations")
	}
}

// TestExternalSuites_ExternalOnly verifies that only external tests run when unit tests are disabled
func TestExternalSuites_ExternalOnly(t *testing.T) {
	configContent := `unit_tests_enabled: false
external_suites:
  enabled: true
  run_mode: only
  suites:
    - name: all-tests
      paths: [./tests/...]
`
	output := runGorgonWithConfig(t, configContent, "./examples/mutations/arithmetic_flip")

	if !strings.Contains(output, "[EXTERNAL] Running external suite phase") {
		t.Error("Expected external suite phase to run")
	}

	if strings.Contains(output, "TestAdd") || strings.Contains(output, "TestSubtract") {
		t.Error("Expected unit tests NOT to run in external-only mode")
	}

	if !strings.Contains(output, "[all-tests]") {
		t.Error("Expected external suite to kill mutations")
	}
}

// TestExternalSuites_UnitOnly verifies that only unit tests run when external tests are disabled
func TestExternalSuites_UnitOnly(t *testing.T) {
	configContent := `unit_tests_enabled: true
external_suites:
  enabled: false
`
	output := runGorgonWithConfig(t, configContent, "./examples/mutations/arithmetic_flip")

	if strings.Contains(output, "[EXTERNAL] Running external suite phase") {
		t.Error("Expected external suite phase to be skipped")
	}

	if strings.Contains(output, "[all-tests]") {
		t.Error("Expected external suite NOT to run in unit-only mode")
	}

	if !strings.Contains(output, "TestAdd") || !strings.Contains(output, "TestSubtract") {
		t.Error("Expected unit tests to run")
	}
}

// TestExternalSuites_NoneEnabled verifies that no tests run when both are disabled
func TestExternalSuites_NoneEnabled(t *testing.T) {
	configContent := `unit_tests_enabled: false
external_suites:
  enabled: false
`
	output := runGorgonWithConfig(t, configContent, "./examples/mutations/arithmetic_flip")

	if strings.Contains(output, "[EXTERNAL] Running external suite phase") {
		t.Error("Expected external suite phase NOT to run")
	}

	if strings.Contains(output, "[all-tests]") {
		t.Error("Expected external suite NOT to run when disabled")
	}

	if strings.Contains(output, "TestAdd") || strings.Contains(output, "TestSubtract") {
		t.Error("Expected unit tests NOT to run")
	}

	if !strings.Contains(output, "0.00%") {
		t.Error("Expected 0% mutation score when no tests run")
	}
}

// runGorgonWithConfig creates a temp config file and runs gorgon with it
func runGorgonWithConfig(t *testing.T, configContent, target string) string {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gorgon.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	projectRoot := findProjectRoot(t)
	gorgonBin := filepath.Join(projectRoot, "gorgon")

	cmd := exec.Command(gorgonBin, "-config="+configPath, target)
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()

	// Check for Schemata compilation failures
	outputStr := string(output)
	if strings.Contains(outputStr, "FATAL: Schemata-transformed code does not compile!") {
		t.Fatalf("Schemata compilation failed:\n%s", outputStr)
	}

	// Check for other critical errors that should fail tests
	if err != nil && strings.Contains(outputStr, "Build errors:") {
		t.Fatalf("Build errors detected:\n%s", outputStr)
	}

	return outputStr
}

// findProjectRoot walks up from current directory to find go.mod
func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (go.mod)")
		}
		dir = parent
	}
}
