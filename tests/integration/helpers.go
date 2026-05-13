//go:build integration


package integration

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/runner"
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






type MutantInfo struct {
	ID       int    `json:"id"`
	Status   string `json:"status"`
	Operator string `json:"operator"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
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

// ============================================================================
// CONFIG-DRIVEN PIPELINE HELPERS
//
// The helpers below accept an external gorgon.yml path (per-test fixture under
// testdata/<TestName>/) plus a real-code target directory inside the repo and
// drive the same engine/runner machinery the gorgon binary uses. They do not
// rely on synthetic fixtures — they mutate production code so the tests
// validate behavior end-to-end.
// ============================================================================

// findRepoRoot walks up from the working directory looking for go.mod and
// returns the absolute path of the repository root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root (no go.mod) above %s", cwd)
		}
		dir = parent
	}
}

// loadIntegrationConfig loads a gorgon.yml file and resolves any relative
// paths inside cfg.Tests against the config file's directory, mirroring the
// convention the binary uses (cmd.Dir == filepath.Dir(configPath)).
func loadIntegrationConfig(t *testing.T, configPath string) *config.Config {
	t.Helper()
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config %s: %v", configPath, err)
	}
	configDir := filepath.Dir(configPath)
	for i, p := range cfg.Tests {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			cfg.Tests[i] = filepath.Join(configDir, p)
		}
	}
	return cfg
}

// generateMutantsRaw runs traversal + GenerateMutants with no filters at all.
// Used as a baseline to confirm a target file/function actually has mutants
// before asserting that a filter removed them.
func generateMutantsRaw(t *testing.T, targetDir string) []coretesting.Mutant {
	t.Helper()
	repoRoot := findRepoRoot(t)
	ops := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)

	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", targetDir, err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites in %s — fixture broken", targetDir)
	}
	log := logger.New(false)
	return coretesting.GenerateMutants(sites, ops, ops, repoRoot, nil, nil, log)
}

// generateMutantsWithConfig loads the config, traverses the target directory,
// applies runner.FilterSites with the config (so skip/exclude/include/skip_func
// take effect), and returns the resulting mutant list. It does NOT execute
// any tests — call this when only filtering behavior matters.
func generateMutantsWithConfig(t *testing.T, configPath, targetDir string) []coretesting.Mutant {
	t.Helper()
	cfg := loadIntegrationConfig(t, configPath)
	repoRoot := findRepoRoot(t)

	ops, err := cli.ParseOperators(cfg)
	if err != nil {
		t.Fatalf("parse operators: %v", err)
	}
	allOps := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	eng.SetSuppressEntries(cfg.Suppress)

	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", targetDir, err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites in %s — fixture broken", targetDir)
	}

	resolver, _ := subconfig.Discover(repoRoot, configPath)
	sites = runner.FilterSites(sites, []string{targetDir}, cfg, resolver)

	log := logger.New(false)
	return coretesting.GenerateMutants(sites, ops, allOps, repoRoot, cfg.DirRules, resolver, log)
}

// runMutantsWithConfig is generateMutantsWithConfig + GenerateAndRunSchemata.
// It compiles the target package once and runs tests against each mutant so
// Status / KilledBy fields are populated. Use this when you need to verify
// kill attribution or the effect of cfg.Tests on mutant statuses.
func runMutantsWithConfig(t *testing.T, configPath, targetDir string) ([]coretesting.Mutant, reporter.ReportStats) {
	t.Helper()
	cfg := loadIntegrationConfig(t, configPath)
	repoRoot := findRepoRoot(t)

	ops, err := cli.ParseOperators(cfg)
	if err != nil {
		t.Fatalf("parse operators: %v", err)
	}
	allOps := mutator.ListAll()
	concurrent := cli.ParseConcurrent(cfg.Concurrent)

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	eng.SetSuppressEntries(cfg.Suppress)

	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", targetDir, err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites in %s — fixture broken", targetDir)
	}

	resolver, _ := subconfig.Discover(repoRoot, configPath)
	sites = runner.FilterSites(sites, []string{targetDir}, cfg, resolver)

	testsByPkg, testPaths := buildTestsByPkg(t, cfg.Tests)

	log := logger.New(false)
	ctx := context.Background()
	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx, sites, ops, allOps,
		targetDir, repoRoot,
		cfg.DirRules, resolver,
		concurrent, nil,
		testsByPkg, testPaths,
		log, false,
		cfg.UnitTestsEnabled, cfg.ExternalSuites, cfg,
	)
	if err != nil {
		t.Logf("pipeline error (some mutants may legitimately fail): %v", err)
	}

	totalMutants := coretesting.GetTotalMutants()
	stats, _ := reporter.Report(
		mutants, totalMutants, 0, resolver,
		false, false, false,
		"", "", "",
		reporter.BaselineOptions{},
	)
	return mutants, stats
}

// buildTestsByPkg mirrors runner.extractTests (which is unexported) so the
// integration tests can drive cfg.Tests through the same filter the binary
// applies. Returns (testsByPkg, testPaths) suitable for GenerateAndRunSchemata.
func buildTestsByPkg(t *testing.T, testPaths []string) (map[string][]string, []string) {
	t.Helper()
	byPkg := make(map[string][]string)
	var paths []string
	for _, p := range testPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("abs %s: %v", p, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		var pkgDir string
		var files []string
		if info.IsDir() {
			pkgDir = abs
			entries, err := os.ReadDir(abs)
			if err != nil {
				t.Fatalf("read dir %s: %v", abs, err)
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
					files = append(files, filepath.Join(abs, e.Name()))
				}
			}
		} else {
			pkgDir = filepath.Dir(abs)
			files = []string{abs}
		}
		for _, f := range files {
			byPkg[pkgDir] = append(byPkg[pkgDir], parseTestNamesFromFile(f)...)
		}
		paths = append(paths, p)
	}
	return byPkg, paths
}

// parseTestNamesFromFile returns the names of all top-level Test* functions
// declared in a Go test file.
func parseTestNamesFromFile(filePath string) []string {
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil
	}
	var names []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if strings.HasPrefix(fn.Name.Name, "Test") {
			names = append(names, fn.Name.Name)
		}
	}
	return names
}

// mutantsByFile groups mutants by source file basename and returns a
// basename -> count map.
func mutantsByFile(mutants []coretesting.Mutant) map[string]int {
	out := make(map[string]int)
	for _, m := range mutants {
		if m.Site.File == nil {
			continue
		}
		out[filepath.Base(m.Site.File.Name())]++
	}
	return out
}

// mutantsInFunction returns the subset of mutants whose enclosing function
// has the given name in the file with the given basename.
func mutantsInFunction(mutants []coretesting.Mutant, fileBasename, funcName string) []coretesting.Mutant {
	var out []coretesting.Mutant
	for _, m := range mutants {
		if m.Site.File == nil {
			continue
		}
		if filepath.Base(m.Site.File.Name()) != fileBasename {
			continue
		}
		if m.Site.FunctionName == funcName {
			out = append(out, m)
		}
	}
	return out
}

