package testing_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemataCompilation(t *testing.T) {
	projectRoot := findProjectRoot(t)
	gorgonBin := filepath.Join(projectRoot, "gorgon")
	
	// Run gorgon on the problematic packages to catch compilation issues
	// Use all operators to catch all potential compilation problems
	cmd := exec.Command(gorgonBin, "-operators=all", "-concurrent=all", "./pkg/mutator", "./pkg/config", "./internal/core", "./internal/reporter")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	
	outputStr := string(output)
	t.Logf("Gorgon output:\n%s", outputStr)
	
	// Check for Schemata compilation failures
	if strings.Contains(outputStr, "FATAL: Schemata-transformed code does not compile!") {
		t.Fatalf("Schemata compilation failed - this indicates bugs in the mutation operators:\n%s", outputStr)
	}
	
	// Check for build errors
	if strings.Contains(outputStr, "Build errors:") {
		t.Fatalf("Build errors detected in Schemata-transformed code:\n%s", outputStr)
	}
	
	// The command might fail for other reasons, but compilation failures are critical
	if err != nil && (strings.Contains(outputStr, "does not compile") || strings.Contains(outputStr, "Build errors")) {
		t.Fatalf("Compilation-related error: %v\nOutput:\n%s", err, outputStr)
	}
}