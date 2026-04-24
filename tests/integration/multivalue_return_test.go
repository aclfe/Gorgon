//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMultiValueReturnMutations is a regression test for the multi-value return bug fix.
//
// Background:
// Prior to the fix, Gorgon's schemata transformation had a bug when handling return
// statements with multiple values (e.g., `return val, err`). The engine would extract
// only the first return type, causing the generated closure to have a mismatched signature:
//
//	func() *config.Config {        // ← returns ONE value
//	    return nil, fmt.Errorf(...) // ← trying to return TWO values
//	}()
//
// This resulted in compilation errors like "not enough return values" or "too many return values",
// causing all mutants to be marked as "untested" because the test binary couldn't be built.
//
// The Fix:
// 1. Engine (internal/engine/engine.go): Extract ALL return types as comma-separated string
// 2. Handler (internal/core/schemata_nodes/handlers.go): Parse and build correct closure signatures
// 3. Validation: Filter out mutations with mismatched return value counts
//
// This test verifies:
// - Test binaries are successfully generated for packages with multi-value returns
// - No "not enough return values" or "too many return values" compilation errors
// - Mutations are properly applied and tested (not all marked "untested")
// - Tests can kill mutations, achieving >50% mutation score
// - The fix remains stable across code changes
func TestMultiValueReturnMutations(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "gorgon_test_multireturn")
	os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sourceFile := filepath.Join(tmpDir, "multireturn.go")
	sourceCode := `package multireturn

import "fmt"

// MultiReturn returns two values - tests the multi-value return fix
func MultiReturn(val int) (int, error) {
	if val < 0 {
		return 0, fmt.Errorf("negative value")
	}
	return val * 2, nil
}
`
	if err := os.WriteFile(sourceFile, []byte(sourceCode), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	testFile := filepath.Join(tmpDir, "multireturn_test.go")
	testCode := `package multireturn

import "testing"

func TestMultiReturn(t *testing.T) {
	result, err := MultiReturn(5)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != 10 {
		t.Errorf("Expected 10, got %d", result)
	}
	
	_, err = MultiReturn(-1)
	if err == nil {
		t.Error("Expected error for negative value")
	}
}
`
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	goModFile := filepath.Join(tmpDir, "go.mod")
	goModContent := `module multireturn

go 1.25
`
	if err := os.WriteFile(goModFile, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	gorgonBin, err := findGorgonBinary()
	if err != nil {
		t.Fatalf("Failed to find gorgon binary: %v", err)
	}

	cmd := exec.Command(gorgonBin, tmpDir)
	output, err := cmd.CombinedOutput()

	outputStr := string(output)
	t.Logf("Gorgon output:\n%s", outputStr)

	// Check for Schemata compilation failures first
	if strings.Contains(outputStr, "FATAL: Schemata-transformed code does not compile!") {
		t.Fatalf("Schemata compilation failed:\n%s", outputStr)
	}

	if err != nil {
		t.Fatalf("Gorgon execution failed: %v", err)
	}

	if strings.Contains(outputStr, "not enough return values") {
		t.Error("Found 'not enough return values' error - multi-value return bug not fixed")
	}
	if strings.Contains(outputStr, "too many return values") {
		t.Error("Found 'too many return values' error - multi-value return bug not fixed")
	}

	if strings.Contains(outputStr, "package has no test files") {
		t.Error("Test binary was not created - compilation likely failed")
	}

	if strings.Contains(outputStr, "Untested") {
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Untested") && strings.Contains(line, "Total") {
				fields := strings.Fields(line)
				if len(fields) >= 6 {
					untested := fields[len(fields)-2]
					total := fields[len(fields)-1]
					if untested == total && untested != "0" {
						t.Errorf("All %s mutants are untested - test binary likely didn't compile", total)
					}
				}
			}
		}
	}

	if !strings.Contains(outputStr, "Mutation Score") {
		t.Error("No mutation score found in output")
	}

	lines := strings.Split(outputStr, "\n")
	foundScore := false
	for _, line := range lines {
		if strings.Contains(line, "Mutation Score") && strings.Contains(line, "Killed") {
			foundScore = true
		} else if foundScore && strings.Contains(line, "%") {
			if strings.HasPrefix(strings.TrimSpace(line), "0.00%") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[1] == "0" {
					t.Error("Mutation score is 0% with 0 killed mutants - tests may not be running properly")
				}
			}
			break
		}
	}

	if !verifyMinimumMutationScore(outputStr, 50.0) {
		t.Error("Mutation score is below 50% - expected tests to kill at least half the mutants")
	}
}

// findGorgonBinary locates the gorgon executable
func findGorgonBinary() (string, error) {
	if _, err := os.Stat("./gorgon"); err == nil {
		return "./gorgon", nil
	}

	if _, err := os.Stat("../../gorgon"); err == nil {
		return "../../gorgon", nil
	}

	cmd := exec.Command("go", "build", "-o", "gorgon", "../../cmd/gorgon/main.go")
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return "./gorgon", nil
}

// verifyMinimumMutationScore checks if mutation score meets minimum threshold
func verifyMinimumMutationScore(output string, minScore float64) bool {
	lines := strings.Split(output, "\n")
	foundHeader := false

	for _, line := range lines {
		if strings.Contains(line, "Mutation Score") && strings.Contains(line, "Killed") {
			foundHeader = true
			continue
		}

		if foundHeader && strings.Contains(line, "%") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				scoreStr := strings.TrimSuffix(fields[0], "%")
				var score float64
				if _, err := fmt.Sscanf(scoreStr, "%f", &score); err == nil {
					return score >= minScore
				}
			}
			break
		}
	}

	return false
}
