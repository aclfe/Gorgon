package testing_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCriticalFilesCompile ensures critical files that are prone to errors
// compile without any compilation issues (undefined vars, unused vars, type errors, etc).
func TestCriticalFilesCompile(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	criticalPackages := []string{
		"pkg/mutator/analysis",
		"internal/gowork",
		"pkg/mutator/tokens",
		"internal/cache",
		"pkg/mutator",
		"pkg/config",
	}

	for _, pkg := range criticalPackages {
		t.Run(pkg, func(t *testing.T) {
			cmd := exec.Command("go", "build", "./"+pkg)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			
			if err != nil {
				outStr := string(output)
				
				var errorType string
				switch {
				case strings.Contains(outStr, "undefined:"):
					errorType = "UNDEFINED VARIABLE"
				case strings.Contains(outStr, "declared and not used"):
					errorType = "UNUSED VARIABLE"
				case strings.Contains(outStr, "cannot use"):
					errorType = "TYPE MISMATCH"
				case strings.Contains(outStr, "invalid operation"):
					errorType = "INVALID OPERATION"
				case strings.Contains(outStr, "too many errors"):
					errorType = "MULTIPLE ERRORS"
				default:
					errorType = "COMPILATION ERROR"
				}
				
				t.Fatalf("%s in package %s:\n%s", errorType, pkg, outStr)
			}
		})
	}
	
	t.Run("all_packages", func(t *testing.T) {
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		
		if err != nil {
			t.Fatalf("Full build failed:\n%s", string(output))
		}
	})
}
