//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// MutantInfo represents a mutant from JSON report
type MutantInfo struct {
	ID       int    `json:"id"`
	Status   string `json:"status"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Operator string `json:"operator,omitempty"`
	KilledBy string `json:"killed_by,omitempty"`
}

// ReportData represents the JSON report structure
type ReportData struct {
	Summary struct {
		Score          float64 `json:"score"`
		Killed         int     `json:"killed"`
		Survived       int     `json:"survived"`
		CompileErrors  int     `json:"compile_errors"`
		RuntimeErrors  int     `json:"runtime_errors"`
		Timeout        int     `json:"timeout"`
		Untested       int     `json:"untested"`
		Invalid        int     `json:"invalid"`
		Total          int     `json:"total"`
		TotalErrors    int     `json:"total_errors"`
	} `json:"summary"`
	Mutants []MutantInfo `json:"mutants"`
}

// runGorgonWithConfig runs gorgon binary with a specific config file
func runGorgonWithConfig(t *testing.T, configPath, targetDir string) (*ReportData, error) {
	t.Helper()

	// Find gorgon binary
	gorgonBinary := os.Getenv("GORGON_BINARY")
	if gorgonBinary == "" {
		// Try to find it in parent directories
		repoRoot, err := findRepoRoot()
		if err != nil {
			return nil, fmt.Errorf("failed to find repo root: %w", err)
		}
		gorgonBinary = filepath.Join(repoRoot, "gorgon")
	}

	// Verify binary exists
	if _, err := os.Stat(gorgonBinary); err != nil {
		return nil, fmt.Errorf("gorgon binary not found at %s: %w", gorgonBinary, err)
	}

	// Build command - output is configured in YAML, not CLI
	args := []string{
		"-config=" + configPath,
		"-progbar=false", // Disable progress bar for cleaner output
		targetDir,
	}

	cmd := exec.Command(gorgonBinary, args...)
	cmd.Dir = filepath.Dir(configPath)

	// Run command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gorgon failed: %w\nOutput: %s", err, string(output))
	}

	// Read JSON output from expected location (specified in config)
	// Config specifies: outputs: ["json:report.json"]
	jsonOutput := filepath.Join(filepath.Dir(configPath), "report.json")
	data, err := os.ReadFile(jsonOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON output from %s: %w", jsonOutput, err)
	}

	var report ReportData
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &report, nil
}

// findRepoRoot finds the repository root by looking for go.mod
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find repo root (no go.mod)")
}
