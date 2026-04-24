package regression

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllPackagesBuild ensures all packages in the repo compile without errors.
// This catches issues like undefined variables, missing imports, type errors, etc.
func TestAllPackagesBuild(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all_packages_build", func(t *testing.T) {
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Build failed:\n%s\nError: %v", string(output), err)
		}
	})

	t.Run("all_packages_vet", func(t *testing.T) {
		cmd := exec.Command("go", "vet", "./...")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go vet failed:\n%s\nError: %v", string(output), err)
		}
	})

	packages := []string{
		"./cmd/...",
		"./internal/...",
		"./pkg/...",
		"./tests/...",
	}
	for _, pkg := range packages {
		t.Run("package_"+strings.TrimSuffix(strings.TrimPrefix(pkg, "./"), "/..."), func(t *testing.T) {
			cmd := exec.Command("go", "build", pkg)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Package %s build failed:\n%s\nError: %v", pkg, string(output), err)
			}
		})
	}
}
