//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

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

// runPipeline runs the full mutation testing pipeline on fixtureDir and returns
// the computed ReportStats. It does not write any output files or to stdout.
func runPipeline(t *testing.T, fixtureDir string) reporter.ReportStats {
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

	// format="" + outputFile="" means computeStats runs but nothing is written.
	stats, _ := reporter.Report(
		mutants,
		totalMutants,
		0,
		nil,
		false,
		false,
		false,
		"",
		"",
		"",
		reporter.BaselineOptions{},
	)
	return stats
}

// runPipelineWithOutputs runs the full mutation testing pipeline and generates
// all output format files for cross-format consistency validation.
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

	// Generate all output formats
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

// ============================================================================
// OUTPUT FORMAT PARSERS
// ============================================================================

// extractStatsFromJSON parses ReportStats from JSON output file.
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

// extractStatsFromJUnit parses ReportStats from JUnit XML output file.
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

// extractStatsFromSARIF parses ReportStats from SARIF output file.
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

// extractStatsFromText parses ReportStats from text output file.
func extractStatsFromText(path string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read text file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Find the stats line with space-separated values
	// Format: "Mutation Score	Killed	Survived	Compile Errors	Runtime Errors	Timeout	Untested	Invalid	Total"
	for i, line := range lines {
		if strings.Contains(line, "Mutation Score") && strings.Contains(line, "Killed") {
			// Next line has the values
			if i+1 < len(lines) {
				valuesLine := lines[i+1]
				// Use Fields to split on whitespace (handles both tabs and spaces)
				values := strings.Fields(valuesLine)
				if len(values) >= 9 {
					// Parse score (remove % suffix)
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

// extractStatsFromHTML parses ReportStats from HTML output file.
func extractStatsFromHTML(dir string) (reporter.ReportStats, error) {
	var stats reporter.ReportStats
	path := filepath.Join(dir, "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		return stats, fmt.Errorf("read html file: %w", err)
	}

	content := string(data)

	// Extract score from: <span class="stat-value score ...">75.0%</span>
	scoreRe := regexp.MustCompile(`<span class="stat-value score [^"]*">([\d.]+)%</span>`)
	if matches := scoreRe.FindStringSubmatch(content); len(matches) > 1 {
		stats.Score, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Extract other stats using a general pattern
	// Format: <span class="stat-label">Killed:</span><span class="stat-value">42</span>
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

// compareStats verifies that two ReportStats have consistent values.
// Returns a list of discrepancies for human-readable error messages.
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

// calculateExpectedScore computes the expected mutation score from category counts.
// Score = Killed / (Killed + Survived + Untested + Timeout) * 100
func calculateExpectedScore(stats reporter.ReportStats) float64 {
	denom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout
	if denom == 0 {
		return 0
	}
	return float64(stats.Killed) / float64(denom) * 100
}
