//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gcore "github.com/aclfe/gorgon/internal/core"
)

// TestExtractMutantIDsFromBuildErrors_Window tests the extractMutantIDsFromBuildErrors function
// with a file that has a compilation error
func TestExtractMutantIDsFromBuildErrors_Window(t *testing.T) {
	// Create a temp directory
	dir := t.TempDir()

	// Create a test file with an activeMutantID marker that will cause a compilation error
	testFile := filepath.Join(dir, "test.go")
	testContent := `package main

func Add(a, b int) int {
	return a + b // activeMutantID == 123
}

func main() {
	_ = UndefinedFunction() // This will cause a compilation error
}
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create a go.mod for the temp directory
	goMod := `module testproject

go 1.25
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Run real go build to get actual compiler output
	cmd := exec.CommandContext(context.Background(), "go", "build", "./...")
	cmd.Dir = dir
	output, _ := cmd.CombinedOutput()

	t.Logf("Build output: %s", string(output))

	// Call the test helper function with real compiler output
	ids := gcore.TestExtractMutantIDsFromBuildErrors(dir, string(output))

	t.Logf("Found IDs: %v", ids)

	// Should find the mutant ID 123 from the source file
	found := false
	for _, id := range ids {
		if id == 123 {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("mutant ID 123 not found in build errors: %v", ids)
	}
}
