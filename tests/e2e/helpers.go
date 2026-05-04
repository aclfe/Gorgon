//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/subconfig"
	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"

	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/concurrency"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/condition_negation"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_panic_recover"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/error_handling"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/function_call_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/logical_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_body_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_first"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/variable_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/zero_value_return"
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

// killStats holds the counts of internal and external kills
type killStats struct {
	InternalKilled   int
	ExternalKilled   int
	InternalKilledBy []string
	ExternalKilledBy []string
}

// debugKillStats logs detailed kill statistics for a report and returns the counts
// logInternal and logExternal control whether to log those categories (set false when disabled)
func debugKillStats(t *testing.T, report *ReportData, testName string, logInternal, logExternal bool) killStats {
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

	if logInternal {
		t.Logf("[%s] INTERNAL KILLED: %d (test names: %v)", testName, stats.InternalKilled, stats.InternalKilledBy[:min(len(stats.InternalKilledBy), 3)])
	}
	if logExternal {
		t.Logf("[%s] EXTERNAL KILLED: %d (suite names: %v)", testName, stats.ExternalKilled, stats.ExternalKilledBy[:min(len(stats.ExternalKilledBy), 3)])
	}
	t.Logf("[%s] Summary: Killed=%d, Survived=%d, Total=%d", testName, report.Summary.Killed, report.Summary.Survived, report.Summary.Total)
	t.Logf("[%s] Status breakdown: CompileErrors=%d, RuntimeErrors=%d, Timeout=%d, Untested=%d, Invalid=%d, TotalErrors=%d",
		testName, report.Summary.CompileErrors, report.Summary.RuntimeErrors, report.Summary.Timeout,
		report.Summary.Untested, report.Summary.Invalid, report.Summary.TotalErrors)

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

// runGorgonWithConfig runs gorgon binary with a specific config file
func runGorgonWithConfig(t *testing.T, configPath, targetDir string) (*ReportData, error) {
	t.Helper()

	// Find gorgon binary
	gorgonBinary := os.Getenv("GORGON_BINARY")
	if gorgonBinary == "" {
		repoRoot, err := findRepoRoot()
		if err != nil {
			return nil, fmt.Errorf("failed to find repo root: %w", err)
		}
		gorgonBinary = filepath.Join(repoRoot, "gorgon")
		// Build binary if missing or stale
		if err := ensureGorgonBinary(t, repoRoot, gorgonBinary); err != nil {
			return nil, fmt.Errorf("failed to build gorgon binary: %w", err)
		}
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

// ensureGorgonBinary builds the gorgon binary if it is missing or stale.
// Staleness is determined by comparing the binary's mtime against all .go
// files under cmd/ and internal/ in the repo root.
func ensureGorgonBinary(t *testing.T, repoRoot, binaryPath string) error {
	t.Helper()

	binInfo, err := os.Stat(binaryPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	needsBuild := err != nil // binary missing
	if !needsBuild {
		binMtime := binInfo.ModTime()
		for _, srcDir := range []string{"cmd", "internal", "pkg"} {
			if stale, _ := dirNewerThan(filepath.Join(repoRoot, srcDir), binMtime); stale {
				needsBuild = true
				break
			}
		}
	}

	if !needsBuild {
		return nil
	}

	t.Log("gorgon binary is missing or stale — rebuilding...")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/gorgon")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %w\n%s", err, out)
	}
	t.Log("gorgon binary rebuilt successfully")
	return nil
}

// dirNewerThan reports whether any .go file under dir has a mtime after ref.
func dirNewerThan(dir string, ref time.Time) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(ref) {
				found = true
			}
		}
		return nil
	})
	return found, err
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

func runPipelineWithOutputs(t *testing.T, fixtureDir string, outputDir string) reporter.ReportStats {
	t.Helper()

	ops := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(fixtureDir)

	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", fixtureDir, err)
	}

	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites found in %s — check fixture", fixtureDir)
	}

	log := logger.New(false)
	resolver, _ := subconfig.Discover(fixtureDir, "")

	ctx := context.Background()
	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx,
		sites,
		ops,
		ops,
		fixtureDir,
		fixtureDir,
		nil,
		resolver,
		runtime.NumCPU(),
		nil,
		nil,
		nil,
		log,
		false,
		true,
		config.ExternalSuitesConfig{},
		&config.Config{},
	)
	if err != nil {
		t.Logf("pipeline error (may be expected for some mutants): %v", err)
	}

	totalMutants := coretesting.GetTotalMutants()

	outputBase := filepath.Join(outputDir, "report")
	stats, _ := reporter.Report(
		mutants,
		totalMutants,
		0,
		nil,
		false,
		false,
		false,
		outputBase+".json",
		"",
		"json",
		reporter.BaselineOptions{
			MultiOutputs: []string{
				"textfile:" + outputBase + ".txt",
				"html:" + outputDir + "/report.html",
				"junit:" + outputBase + ".xml",
				"sarif:" + outputBase + ".sarif",
			},
		},
	)
	return stats
}

func extractStatsFromJSON(path string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read json file: %w", err)
	}

	var report struct {
		Summary reporter.ReportStats `json:"summary"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return stats, fmt.Errorf("unmarshal json: %w", err)
	}
	return report.Summary, nil
}

func extractStatsFromJUnit(path string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read junit file: %w", err)
	}

	var suite struct {
		reporter.ReportStats
	}
	if err := xml.Unmarshal(data, &suite); err != nil {
		return stats, fmt.Errorf("unmarshal junit xml: %w", err)
	}
	return suite.ReportStats, nil
}

func extractStatsFromSARIF(path string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read sarif file: %w", err)
	}

	var report struct {
		Runs []struct {
			Properties reporter.ReportStats `json:"properties"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return stats, fmt.Errorf("unmarshal sarif json: %w", err)
	}

	if len(report.Runs) == 0 {
		return stats, fmt.Errorf("no runs found in sarif")
	}
	return report.Runs[0].Properties, nil
}

func extractStatsFromText(path string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read text file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if strings.Contains(line, "Mutation Score") && strings.Contains(line, "Killed") {

			if i+1 < len(lines) {
				valuesLine := lines[i+1]

				values := strings.Fields(valuesLine)
				if len(values) >= 9 {

					scoreStr := strings.TrimSuffix(values[0], "%")
					stats.Score, _ = strconv.ParseFloat(scoreStr, 64)
					stats.Killed, _ = strconv.Atoi(values[1])
					stats.Survived, _ = strconv.Atoi(values[2])
					stats.CompileErrors, _ = strconv.Atoi(values[3])
					stats.RuntimeErrors, _ = strconv.Atoi(values[4])
					stats.Timeout, _ = strconv.Atoi(values[5])
					stats.Untested, _ = strconv.Atoi(values[6])
					stats.Invalid, _ = strconv.Atoi(values[7])
					stats.Total, _ = strconv.Atoi(values[8])
					stats.TotalErrors = stats.CompileErrors + stats.RuntimeErrors
				}
				break
			}
		}
	}

	return stats, nil
}

func extractStatsFromHTML(dir string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	path := filepath.Join(dir, "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read html file: %w", err)
	}

	content := string(data)

	scoreRe := regexp.MustCompile(`<span class="stat-value score [^"]*">([\d.]+)%</span>`)
	if matches := scoreRe.FindStringSubmatch(content); len(matches) > 1 {
		stats.Score, _ = strconv.ParseFloat(matches[1], 64)
	}

	extractStat := func(label string) int {
		re := regexp.MustCompile(`<span class="stat-label">` + label + `:</span>\s*<span class="stat-value">(\d+)</span>`)
		if matches := re.FindStringSubmatch(content); len(matches) > 1 {
			val, _ := strconv.Atoi(matches[1])
			return val
		}
		return 0
	}

	stats.Killed = extractStat("Killed")
	stats.Survived = extractStat("Survived")
	stats.CompileErrors = extractStat("Compile Errors")
	stats.RuntimeErrors = extractStat("Runtime Errors")
	stats.Timeout = extractStat("Timeout")
	stats.Untested = extractStat("Untested")
	stats.Invalid = extractStat("Invalid")
	stats.Total = extractStat("Total")
	stats.TotalErrors = stats.CompileErrors + stats.RuntimeErrors

	return stats, nil
}

func compareStats(a, b reporter.ReportStats, formatName string) []string {
	var discrepancies []string

	if math.Abs(a.Score-b.Score) > 0.01 {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Score: %s=%.2f, expected=%.2f", formatName, b.Score, a.Score))
	}
	if a.Killed != b.Killed {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Killed: %s=%d, expected=%d", formatName, b.Killed, a.Killed))
	}
	if a.Survived != b.Survived {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Survived: %s=%d, expected=%d", formatName, b.Survived, a.Survived))
	}
	if a.CompileErrors != b.CompileErrors {
		discrepancies = append(discrepancies,
			fmt.Sprintf("CompileErrors: %s=%d, expected=%d", formatName, b.CompileErrors, a.CompileErrors))
	}
	if a.RuntimeErrors != b.RuntimeErrors {
		discrepancies = append(discrepancies,
			fmt.Sprintf("RuntimeErrors: %s=%d, expected=%d", formatName, b.RuntimeErrors, a.RuntimeErrors))
	}
	if a.TotalErrors != b.TotalErrors {
		discrepancies = append(discrepancies,
			fmt.Sprintf("TotalErrors: %s=%d, expected=%d", formatName, b.TotalErrors, a.TotalErrors))
	}
	if a.Timeout != b.Timeout {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Timeout: %s=%d, expected=%d", formatName, b.Timeout, a.Timeout))
	}
	if a.Untested != b.Untested {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Untested: %s=%d, expected=%d", formatName, b.Untested, a.Untested))
	}
	if a.Invalid != b.Invalid {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Invalid: %s=%d, expected=%d", formatName, b.Invalid, a.Invalid))
	}
	if a.Total != b.Total {
		discrepancies = append(discrepancies,
			fmt.Sprintf("Total: %s=%d, expected=%d", formatName, b.Total, a.Total))
	}

	return discrepancies
}

func calculateExpectedScore(stats reporter.ReportStats) float64 {
	denom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout
	if denom == 0 {
		return 0
	}
	return float64(stats.Killed) / float64(denom) * 100
}

func runPipelineWithMutantTracking(t *testing.T, fixtureDir string, outputDir string) ([]MutantInfo, reporter.ReportStats) {
	t.Helper()

	ops := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(fixtureDir)

	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", fixtureDir, err)
	}

	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites found in %s — check fixture", fixtureDir)
	}

	log := logger.New(false)
	resolver, _ := subconfig.Discover(fixtureDir, "")

	ctx := context.Background()
	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx,
		sites,
		ops,
		ops,
		fixtureDir,
		fixtureDir,
		nil,
		resolver,
		runtime.NumCPU(),
		nil,
		nil,
		nil,
		log,
		false,
		true,
		config.ExternalSuitesConfig{},
		&config.Config{},
	)
	if err != nil {
		t.Logf("pipeline error (may be expected for some mutants): %v", err)
	}

	totalMutants := coretesting.GetTotalMutants()

	outputBase := filepath.Join(outputDir, "report")
	stats, _ := reporter.Report(
		mutants,
		totalMutants,
		0,
		nil,
		false,
		false,
		false,
		outputBase+".json",
		"",
		"json",
		reporter.BaselineOptions{
			MultiOutputs: []string{
				"textfile:" + outputBase + ".txt",
				"html:" + outputDir + "/report.html",
				"junit:" + outputBase + ".xml",
				"sarif:" + outputBase + ".sarif",
			},
		},
	)

	mutantInfos := make([]MutantInfo, 0, len(mutants))
	for _, m := range mutants {
		info := MutantInfo{
			ID:       m.ID,
			Status:   m.Status,
			Operator: m.Operator.Name(),
		}
		if m.Site.File != nil {
			info.File = m.Site.File.Name()
			info.Line = m.Site.Line
			info.Column = m.Site.Column
		}
		mutantInfos = append(mutantInfos, info)
	}

	return mutantInfos, stats
}

func extractMutantsFromJSON(path string) ([]MutantInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read json file: %w", err)
	}

	var report struct {
		Mutants []MutantInfo `json:"mutants"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	idCount := make(map[int]int)
	for _, m := range report.Mutants {
		idCount[m.ID]++
	}
	var duplicates []string
	for id, count := range idCount {
		if count > 1 {
			duplicates = append(duplicates, fmt.Sprintf("ID %d appears %d times", id, count))
		}
	}
	if len(duplicates) > 0 {
		return nil, fmt.Errorf("duplicate mutants in JSON: %s", strings.Join(duplicates, ", "))
	}

	return report.Mutants, nil
}

func extractMutantsFromHTML(dir string) ([]MutantInfo, error) {
	path := filepath.Join(dir, "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read html file: %w", err)
	}

	content := string(data)
	var mutants []MutantInfo

	idRe := regexp.MustCompile(`"ID":\s*(\d+)`)
	matches := idRe.FindAllStringSubmatch(content, -1)

	seen := make(map[int]bool)
	for _, match := range matches {
		if len(match) > 1 {
			id, _ := strconv.Atoi(match[1])
			if id > 0 && !seen[id] {
				seen[id] = true
				mutants = append(mutants, MutantInfo{ID: id})
			}
		}
	}

	return mutants, nil
}

func extractMutantsFromText(path string) ([]MutantInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read text file: %w", err)
	}

	content := string(data)
	var mutants []MutantInfo

	idRe := regexp.MustCompile(`#(\d+)(?:\s+|$)`)
	matches := idRe.FindAllStringSubmatch(content, -1)

	seen := make(map[int]bool)
	for _, match := range matches {
		if len(match) > 1 {
			id, _ := strconv.Atoi(match[1])
			if id > 0 && !seen[id] {
				seen[id] = true
				mutants = append(mutants, MutantInfo{ID: id})
			}
		}
	}

	return mutants, nil
}

func validateMutantStatus(status string) error {
	validStatuses := map[string]bool{
		"killed":         true,
		"survived":       true,
		"error":          true,
		"timeout":        true,
		"untested":       true,
		"invalid":        true,
		"compile_error":  true,
		"runtime_error":  true,
	}

	if status == "" {
		return fmt.Errorf("status is empty")
	}

	normalized := strings.ToLower(strings.ReplaceAll(status, " ", "_"))
	if !validStatuses[normalized] && !validStatuses[status] {
		return fmt.Errorf("invalid status: %q", status)
	}
	return nil
}

func validateMutant(m MutantInfo) []string {
	var errors []string

	if m.ID <= 0 {
		errors = append(errors, fmt.Sprintf("invalid ID: %d", m.ID))
	}

	if m.File == "" {
		errors = append(errors, fmt.Sprintf("mutant %d: file is empty", m.ID))
	}

	if m.Line <= 0 {
		errors = append(errors, fmt.Sprintf("mutant %d: invalid line: %d", m.ID, m.Line))
	}

	if m.Column <= 0 {
		errors = append(errors, fmt.Sprintf("mutant %d: invalid column: %d", m.ID, m.Column))
	}

	if m.Operator == "" {
		errors = append(errors, fmt.Sprintf("mutant %d: operator is empty", m.ID))
	}

	if err := validateMutantStatus(m.Status); err != nil {
		errors = append(errors, fmt.Sprintf("mutant %d: %v", m.ID, err))
	}

	return errors
}

func checkIDCompleteness(mutants []MutantInfo) []string {
	var errors []string

	if len(mutants) == 0 {
		return []string{"no mutants found"}
	}

	maxID := 0
	for _, m := range mutants {
		if m.ID > maxID {
			maxID = m.ID
		}
	}

	idCount := make(map[int]int)
	for _, m := range mutants {
		idCount[m.ID]++
	}

	for i := 1; i <= maxID; i++ {
		count, ok := idCount[i]
		if !ok {
			errors = append(errors, fmt.Sprintf("missing mutant ID: %d", i))
		} else if count > 1 {
			errors = append(errors, fmt.Sprintf("duplicate mutant ID: %d (appears %d times)", i, count))
		}
	}

	return errors
}

func compareMutantLists(expected, actual []MutantInfo, formatName string) []string {
	var discrepancies []string

	expectedMap := make(map[int]MutantInfo)
	for _, m := range expected {
		expectedMap[m.ID] = m
	}

	actualMap := make(map[int]MutantInfo)
	for _, m := range actual {
		actualMap[m.ID] = m
	}

	for id, exp := range expectedMap {
		if _, ok := actualMap[id]; !ok {
			discrepancies = append(discrepancies,
				fmt.Sprintf("missing mutant ID %d (file=%s, line=%d) in %s", id, exp.File, exp.Line, formatName))
		}
	}

	for id, act := range actualMap {
		if _, ok := expectedMap[id]; !ok {
			discrepancies = append(discrepancies,
				fmt.Sprintf("extra mutant ID %d (file=%s, line=%d) in %s", id, act.File, act.Line, formatName))
		}
	}

	return discrepancies
}
