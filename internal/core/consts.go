package testing

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Go version used in generated go.mod files.
// Auto-detected from project's go.mod, falls back to "1.25" if detection fails.
var goVersion = detectGoVersion()

// detectGoVersion reads the project's go.mod to extract the Go version.
func detectGoVersion() string {
	// Try to find go.mod in current directory or parent directories
	dir, _ := os.Getwd()
	for dir != "" && dir != "/" && dir != "." {
		goModPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			// Extract "go X.Y" line
			re := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+)`)
			if match := re.FindSubmatch(data); len(match) > 1 {
				return string(match[1])
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback to current stable version
	return "1.25"
}

// SetGoVersion allows overriding the detected Go version (for testing or config).
func SetGoVersion(version string) {
	if version != "" && strings.HasPrefix(version, "1.") {
		goVersion = version
	}
}

// Default module name used in standalone benchmarks.
const defaultModuleName = "gorgon-standalone"

// Default module name used in bench configurations.
const benchModuleName = "gorgon-bench"

// Timeout multiplier for mutant test execution relative to baseline.
const timeoutMultiplier = 3.0

// Maximum timeout for mutant test execution.
const maxTimeout = 30

// Minimum baseline duration for timeout calculations (ms).
const minBaselineDuration = 100

// Maximum baseline duration to prevent inflated per-mutant timeouts (5s).
const maxBaselineCap = 5 * 1e9 // 5 seconds in nanoseconds

// Minimum per-mutant timeout (500ms) — prevents fast tests from being killed.
const minMutantTimeout = 500 * 1e6 // 500ms in nanoseconds

// Default per-mutant timeout used when baseline measurement isn't available
// (e.g., the package has no test files so the binary exits immediately).
// 10s is generous enough for most test suites without hanging indefinitely.
const defaultMutantTimeout = 10 * time.Second

// Extra margin added to hard timeout beyond -test.timeout flag.
const hardTimeoutMargin = 2e9 // 2 seconds in nanoseconds

// File permissions for generated files.
const filePermissions = 0o600
