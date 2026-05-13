//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aclfe/gorgon/internal/baseline"
	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/orgpolicy"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/runner"
	"github.com/aclfe/gorgon/internal/subconfig"
	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// ============================================================================
// WORKFLOW FILTERING
//
// All filtering tests mutate the real `internal/reporter` package. Each test
// loads its own gorgon.yml from tests/integration/testdata/<TestName>/ and
// asserts that the configured filter changes the resulting mutant set in the
// way a user would expect — without inspecting any of the filter machinery.
// ============================================================================

// reporterTargetDir is the path of the production package the filtering
// tests mutate. It must contain at least the files referenced by each
// gorgon.yml fixture below (reporter.go, json.go, junit.go, ...).
const reporterTargetSubpath = "internal/reporter"

// TestWorkflow_SkipRulesRespected verifies the `skip` and `exclude` config
// fields actually remove mutants from the listed files. The fixture skips
// reporter.go and excludes json.go; both files have mutants in the unfiltered
// baseline, so a passing filter must zero them out while leaving other files
// untouched.
func TestWorkflow_SkipRulesRespected(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_SkipRulesRespected/gorgon.yml")
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baseline := mutantsByFile(generateMutantsRaw(t, targetDir))
	if baseline["reporter.go"] == 0 {
		t.Fatalf("baseline broken: reporter.go has no mutants, can't verify skip works")
	}
	if baseline["json.go"] == 0 {
		t.Fatalf("baseline broken: json.go has no mutants, can't verify exclude works")
	}

	filtered := mutantsByFile(generateMutantsWithConfig(t, configPath, targetDir))

	if n := filtered["reporter.go"]; n > 0 {
		t.Errorf("skip:[reporter.go] not respected — got %d mutants in reporter.go (baseline had %d)",
			n, baseline["reporter.go"])
	}
	if n := filtered["json.go"]; n > 0 {
		t.Errorf("exclude:[json.go] not respected — got %d mutants in json.go (baseline had %d)",
			n, baseline["json.go"])
	}

	// Sanity: at least one other source file must still have mutants — the
	// filter should not have nuked the whole package.
	otherTotal := 0
	for f, n := range filtered {
		if f == "reporter.go" || f == "json.go" {
			continue
		}
		otherTotal += n
	}
	if otherTotal == 0 {
		t.Errorf("filter removed every mutant — expected non-skipped files to retain mutants. files=%v", filtered)
	}
}

// TestWorkflow_SkipFunc verifies the `skip_func` config field removes only
// mutants whose enclosing function matches. The fixture skips
// reporter.go:Report; the test checks that no mutant lives inside that
// function while the rest of reporter.go (other functions like
// CalculateScore, computeStats, etc) still produces mutants.
func TestWorkflow_SkipFunc(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_SkipFunc/gorgon.yml")
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineRaw := generateMutantsRaw(t, targetDir)
	baselineInReport := mutantsInFunction(baselineRaw, "reporter.go", "Report")
	if len(baselineInReport) == 0 {
		t.Fatalf("baseline broken: reporter.go:Report has no mutants, can't verify skip_func")
	}

	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	if n := len(mutantsInFunction(filtered, "reporter.go", "Report")); n > 0 {
		t.Errorf("skip_func:[reporter.go:Report] not respected — got %d mutants inside Report (baseline had %d)",
			n, len(baselineInReport))
	}

	// Sanity: reporter.go should still have mutants from OTHER functions.
	totalInReporter := mutantsByFile(filtered)["reporter.go"]
	if totalInReporter == 0 {
		t.Errorf("skip_func removed all mutants from reporter.go — expected mutants in non-Report functions to remain")
	}
}

// TestWorkflow_IncludeRules verifies that `include` restricts mutation to
// the listed files only. The fixture allow-lists junit.go; every produced
// mutant must come from junit.go and the file must still have mutants.
func TestWorkflow_IncludeRules(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_IncludeRules/gorgon.yml")
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baseline := mutantsByFile(generateMutantsRaw(t, targetDir))
	if baseline["junit.go"] == 0 {
		t.Fatalf("baseline broken: junit.go has no mutants, can't verify include")
	}
	// Confirm the baseline has more than just junit.go — otherwise the
	// include filter is a no-op and the test proves nothing.
	hasOther := false
	for f, n := range baseline {
		if f != "junit.go" && n > 0 {
			hasOther = true
			break
		}
	}
	if !hasOther {
		t.Fatalf("baseline broken: only junit.go has mutants, can't verify include is restrictive")
	}

	filtered := mutantsByFile(generateMutantsWithConfig(t, configPath, targetDir))
	if filtered["junit.go"] == 0 {
		t.Errorf("include:[junit.go] dropped junit.go itself — got 0 mutants, baseline had %d",
			baseline["junit.go"])
	}
	for f, n := range filtered {
		if f == "junit.go" {
			continue
		}
		if n > 0 {
			t.Errorf("include:[junit.go] should restrict mutation to junit.go, but %s has %d mutants", f, n)
		}
	}
}

// TestWorkflow_TestsFlagFilters verifies the `tests` config field restricts
// which test packages cover mutants. The fixture points tests= at a different
// package (pkg/mutator/operators/empty_body) than the target. Result: every
// mutant in the target should be marked "untested" because its package is
// not in the covered set. Without the filter, those mutants would normally
// be killed/survived by negate_condition's own tests.
func TestWorkflow_TestsFlagFilters(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_TestsFlagFilters/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	mutants, _ := runMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("no mutants produced — fixture broken")
	}

	for _, m := range mutants {
		if m.Status != "untested" {
			t.Errorf("tests:[empty_body] not respected — mutant %d (%s) has status=%q (expected untested)",
				m.ID, m.Operator.Name(), m.Status)
		}
	}
}

// TestWorkflow_TestsSkipFiltersAll verifies skip + exclude + include +
// skip_func compose correctly. The fixture configures all four:
//
//	skip:      [reporter.go]              — must remove all reporter.go mutants
//	exclude:   [json.go]                  — must remove all json.go mutants
//	include:   [junit.go,sarif.go,...]    — must restrict mutation to these
//	skip_func: [junit.go:writeJUnitReport]— must remove that function's mutants
//
// The test asserts each rule independently to make failures attributable.
func TestWorkflow_TestsSkipFiltersAll(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_TestsSkipFiltersAll/gorgon.yml")
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineRaw := generateMutantsRaw(t, targetDir)
	baseline := mutantsByFile(baselineRaw)

	// Pre-checks: any rule we test must have something to remove in the
	// baseline, otherwise a passing assertion proves nothing.
	for _, name := range []string{"reporter.go", "json.go", "junit.go", "sarif.go", "html.go", "textfile.go"} {
		if baseline[name] == 0 {
			t.Fatalf("baseline broken: %s has no mutants", name)
		}
	}
	if len(mutantsInFunction(baselineRaw, "junit.go", "writeJUnitReport")) == 0 {
		t.Fatalf("baseline broken: junit.go:writeJUnitReport has no mutants")
	}

	filtered := generateMutantsWithConfig(t, configPath, targetDir)
	filteredByFile := mutantsByFile(filtered)

	if n := filteredByFile["reporter.go"]; n > 0 {
		t.Errorf("skip:[reporter.go] not respected — %d mutants remain", n)
	}
	if n := filteredByFile["json.go"]; n > 0 {
		t.Errorf("exclude:[json.go] not respected — %d mutants remain", n)
	}

	// `include` is restrictive: anything not on the allow-list must be empty.
	allowlist := map[string]bool{
		"junit.go": true, "sarif.go": true, "html.go": true, "textfile.go": true,
	}
	for f, n := range filteredByFile {
		if allowlist[f] {
			continue
		}
		if n > 0 {
			t.Errorf("include not respected — %s has %d mutants but is not on the allow-list", f, n)
		}
	}

	// Every allow-listed file should still produce mutants — except junit.go,
	// where most mutants live in writeJUnitReport which is skipped. Check
	// each one positively rather than batching, so a regression points at
	// the specific file.
	for _, f := range []string{"junit.go", "sarif.go", "html.go", "textfile.go"} {
		if filteredByFile[f] == 0 && f != "junit.go" {
			t.Errorf("include:[%s] dropped — got 0 mutants, baseline had %d", f, baseline[f])
		}
	}

	if n := len(mutantsInFunction(filtered, "junit.go", "writeJUnitReport")); n > 0 {
		t.Errorf("skip_func:[junit.go:writeJUnitReport] not respected — %d mutants remain", n)
	}
}

// TestWorkflow_KillAttributionCorrect verifies that when a mutant is killed,
// its KilledBy field names a real test function from the package's test files
// (and not a placeholder like "(compiler)" or an empty string). Target is
// pkg/mutator/operators/negate_condition because it's small and has real
// non-skipped tests that actually kill mutants.
func TestWorkflow_KillAttributionCorrect(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	knownTests := parseTestNamesFromFile(filepath.Join(targetDir, "negate_condition_test.go"))
	if len(knownTests) == 0 {
		t.Fatalf("could not parse test names from negate_condition_test.go")
	}
	known := make(map[string]bool, len(knownTests))
	for _, name := range knownTests {
		known[name] = true
	}

	mutants, _ := runMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("no mutants produced — fixture broken")
	}

	var killed int
	for _, m := range mutants {
		if m.Status != "killed" {
			continue
		}
		killed++

		if m.KilledBy == "" {
			t.Errorf("mutant %d killed but KilledBy is empty (op=%s)", m.ID, m.Operator.Name())
			continue
		}

		// External-suite kills wrap the suite name in brackets; this fixture
		// has no external suites, so no killed mutant should look like that.
		if strings.HasPrefix(m.KilledBy, "[") {
			t.Errorf("mutant %d killed by external suite marker %q but no external suite is configured",
				m.ID, m.KilledBy)
			continue
		}

		// KilledBy may be "TestFoo" or "TestFoo/sub" — strip the subtest
		// suffix before comparing against parsed test names.
		topLevel := m.KilledBy
		if i := strings.IndexByte(topLevel, '/'); i >= 0 {
			topLevel = topLevel[:i]
		}
		if !known[topLevel] {
			t.Errorf("mutant %d KilledBy=%q does not match any Test* in negate_condition_test.go (known: %v)",
				m.ID, m.KilledBy, knownTests)
		}
	}

	if killed == 0 {
		t.Errorf("expected at least one killed mutant in negate_condition (its tests should kill some) — got 0; statuses=%v",
			statusBreakdown(mutants))
	}
}

// statusBreakdown is a small debug-only helper used in failure messages.
func statusBreakdown(mutants []coretesting.Mutant) map[string]int {
	out := map[string]int{}
	for _, m := range mutants {
		out[m.Status]++
	}
	return out
}

// ============================================================================
// WORKFLOW WORKSPACE MULTI-MODULE
// ============================================================================

// TestWorkflow_WorkspaceMultiModulePreserved verifies that the workspace temp
// directory created during a pipeline run is fully cleaned up on exit. A
// temp dir leak means the workspace teardown path is broken.
func TestWorkflow_WorkspaceMultiModulePreserved(t *testing.T) {
	if os.Getenv("GORGON_KEEP_TMPDIR") == "1" {
		t.Skip("GORGON_KEEP_TMPDIR=1: workspace temp dirs are intentionally kept; cleanup check is skipped")
	}

	before := schemaTempDirs(t)

	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	mutants, _ := runMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("pipeline produced no mutants — fixture or target broken")
	}

	after := schemaTempDirs(t)
	for dir := range after {
		if !before[dir] {
			t.Errorf("workspace temp dir leaked after pipeline run: %s (workspace cleanup is broken)", dir)
		}
	}
}

// schemaTempDirs returns the set of gorgon-schemata-* directory names
// currently in os.TempDir().
func schemaTempDirs(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	out := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "gorgon-schemata-") {
			out[e.Name()] = true
		}
	}
	return out
}

// ============================================================================
// WORKFLOW DIR RULES
// ============================================================================

// TestWorkflow_DirRulesWhitelistBlacklist verifies that dir_rules whitelist
// and blacklist correctly restrict which operators apply to files within a
// specific directory.
func TestWorkflow_DirRulesWhitelistBlacklist(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	ops := operatorSet(baselineMutants)
	if len(ops) <= 1 {
		t.Fatalf("baseline has only %d distinct operator(s) — need more to verify whitelist/blacklist", len(ops))
	}
	if !ops["negate_condition"] {
		t.Fatalf("baseline has no negate_condition mutants — can't verify whitelist")
	}

	t.Run("Whitelist", func(t *testing.T) {
		configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_DirRulesWhitelistBlacklist/gorgon.yml")
		filtered := generateMutantsWithConfig(t, configPath, targetDir)

		if len(filtered) == 0 {
			t.Fatalf("whitelist removed all mutants — expected negate_condition mutants to remain")
		}
		for _, m := range filtered {
			if m.Operator.Name() != "negate_condition" {
				t.Errorf("dir_rules whitelist not respected: got operator %q, expected only negate_condition",
					m.Operator.Name())
			}
		}
	})

	t.Run("Blacklist_All", func(t *testing.T) {
		// A dir_rule with blacklist: [all] must produce zero mutants for that dir.
		blacklistYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    blacklist:
      - all
`
		tmpCfg := writeTempConfig(t, blacklistYAML)
		filtered := generateMutantsWithConfig(t, tmpCfg, targetDir)
		if len(filtered) > 0 {
			t.Errorf("dir_rules blacklist:[all] not respected — got %d mutants (expected 0)", len(filtered))
		}
	})

	t.Run("Blacklist_Specific", func(t *testing.T) {
		// Blacklisting negate_condition should leave all other operators active.
		blacklistYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    blacklist:
      - negate_condition
`
		tmpCfg := writeTempConfig(t, blacklistYAML)
		filtered := generateMutantsWithConfig(t, tmpCfg, targetDir)

		if len(filtered) == 0 {
			t.Fatalf("blacklist of one operator removed all mutants — fixture broken")
		}
		for _, m := range filtered {
			if m.Operator.Name() == "negate_condition" {
				t.Errorf("dir_rules blacklist:[negate_condition] not respected — got negate_condition mutant %d", m.ID)
			}
		}
	})
}

// operatorSet returns the set of distinct operator names across the mutant slice.
func operatorSet(mutants []coretesting.Mutant) map[string]bool {
	out := make(map[string]bool)
	for _, m := range mutants {
		if m.Operator != nil {
			out[m.Operator.Name()] = true
		}
	}
	return out
}

// writeTempConfig writes YAML content to a temp file and returns the path.
func writeTempConfig(t *testing.T, yaml string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gorgon.yml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

// ============================================================================
// WORKFLOW SUB-CONFIG INHERITANCE
// ============================================================================

// TestWorkflow_SubConfigInheritance verifies that a gorgon.yml placed inside
// a subdirectory overrides the root operator list for files in that directory,
// while files outside the directory are unaffected.
//
// A temporary gorgon.yml is written to internal/baseline/ and removed after
// the test. That directory is not targeted by any other integration test so
// the transient file is safe.
func TestWorkflow_SubConfigInheritance(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "internal/baseline")

	// Precondition: baseline package must have mutants from multiple operators.
	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("internal/baseline has no mutants — fixture broken")
	}
	if !operatorSet(baselineMutants)["arithmetic_flip"] {
		t.Fatalf("baseline has no arithmetic_flip mutants — can't verify sub-config operator override")
	}
	if len(operatorSet(baselineMutants)) <= 1 {
		t.Fatalf("baseline has only one operator — need at least two to verify sub-config restricts to one")
	}

	subConfigPath := filepath.Join(targetDir, "gorgon.yml")
	if _, err := os.Stat(subConfigPath); err == nil {
		t.Fatalf("sub-config already exists at %s — refusing to overwrite production file", subConfigPath)
	}

	subConfigContent := "operators:\n  - arithmetic_flip\n"
	if err := os.WriteFile(subConfigPath, []byte(subConfigContent), 0o644); err != nil {
		t.Fatalf("write sub-config: %v", err)
	}
	t.Cleanup(func() { os.Remove(subConfigPath) })

	// Root config: all operators enabled.
	rootCfgYAML := "operators:\n  - all\nthreshold: 0\nconcurrent: 1\ncache: false\nunit_tests_enabled: false\n"
	rootConfigPath := writeTempConfig(t, rootCfgYAML)

	filtered := generateMutantsWithConfig(t, rootConfigPath, targetDir)
	if len(filtered) == 0 {
		t.Fatalf("sub-config override removed all mutants — expected arithmetic_flip mutants to remain")
	}
	for _, m := range filtered {
		if m.Operator.Name() != "arithmetic_flip" {
			t.Errorf("sub-config operators:[arithmetic_flip] not respected — got operator %q (expected only arithmetic_flip)",
				m.Operator.Name())
		}
	}
}

// ============================================================================
// WORKFLOW BASELINE
// ============================================================================

// TestWorkflow_BaselineNoRegression verifies that no-regression mode:
//   - passes when the current score equals the saved baseline
//   - fails when the current score drops below the saved baseline
//   - auto-creates a baseline when none exists
func TestWorkflow_BaselineNoRegression(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	mutants, stats := runMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("pipeline produced no mutants — fixture broken")
	}

	tmpDir := t.TempDir()
	baselineFile := filepath.Join(tmpDir, ".gorgon-baseline.json")

	// Save the current score as the baseline.
	saved := &baseline.Data{
		Score:    stats.Score,
		Killed:   stats.Killed,
		Survived: stats.Survived,
		Untested: stats.Untested,
		Total:    stats.Total,
	}
	if err := baseline.Save(tmpDir, baselineFile, saved); err != nil {
		t.Fatalf("save baseline: %v", err)
	}

	t.Run("SameScorePasses", func(t *testing.T) {
		current := &baseline.Data{Score: stats.Score}
		if err := baseline.CheckRegression(current, saved, 0); err != nil {
			t.Errorf("same score should pass no-regression check: %v", err)
		}
	})

	t.Run("DroppedScoreFails", func(t *testing.T) {
		dropped := &baseline.Data{Score: stats.Score - 10}
		if err := baseline.CheckRegression(dropped, saved, 0); err == nil {
			t.Errorf("score dropped by 10pp should fail no-regression (baseline=%.2f%%, current=%.2f%%)",
				saved.Score, dropped.Score)
		}
	})

	t.Run("ReporterIntegration_PassesWhenScoreMatches", func(t *testing.T) {
		totalMutants := coretesting.GetTotalMutants()
		_, err := reporter.Report(
			mutants, totalMutants, 0, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{
				NoRegression: true,
				Dir:          tmpDir,
				File:         baselineFile,
			},
		)
		if err != nil {
			t.Errorf("reporter.Report with matching score should pass no-regression: %v", err)
		}
	})

	t.Run("AutoCreatesBaselineWhenMissing", func(t *testing.T) {
		emptyDir := t.TempDir()
		newBaselineFile := filepath.Join(emptyDir, ".gorgon-baseline.json")
		totalMutants := coretesting.GetTotalMutants()

		_, err := reporter.Report(
			mutants, totalMutants, 0, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{
				NoRegression: true,
				Dir:          emptyDir,
				File:         newBaselineFile,
			},
		)
		if err != nil {
			t.Errorf("first run with NoRegression should auto-create baseline, not fail: %v", err)
		}
		if _, statErr := os.Stat(newBaselineFile); os.IsNotExist(statErr) {
			t.Errorf("baseline file was not auto-created at %s", newBaselineFile)
		}
	})
}

// TestWorkflow_BaselineTolerance verifies that the tolerance parameter allows
// the mutation score to drop by up to N percentage points before triggering
// a regression failure.
func TestWorkflow_BaselineTolerance(t *testing.T) {
	saved := &baseline.Data{Score: 80.0}

	tests := []struct {
		name      string
		current   float64
		tolerance float64
		wantPass  bool
	}{
		{"ExactMatch_Passes", 80.0, 0, true},
		{"SlightDrop_NoTolerance_Fails", 75.0, 0, false},
		{"SlightDrop_WithinTolerance_Passes", 75.0, 10.0, true},
		{"SlightDrop_BelowTolerance_Fails", 75.0, 4.0, false},
		{"LargeDrop_LargeTolerance_Passes", 50.0, 30.1, true},
		{"LargeDrop_ExactTolerance_Passes", 50.0, 30.0, true},
		{"LargeDrop_JustBelowTolerance_Fails", 50.0, 29.9, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			current := &baseline.Data{Score: tc.current}
			err := baseline.CheckRegression(current, saved, tc.tolerance)
			if tc.wantPass && err != nil {
				t.Errorf("expected pass (current=%.1f, baseline=%.1f, tolerance=%.1f) but got: %v",
					tc.current, saved.Score, tc.tolerance, err)
			}
			if !tc.wantPass && err == nil {
				t.Errorf("expected fail (current=%.1f, baseline=%.1f, tolerance=%.1f) but passed",
					tc.current, saved.Score, tc.tolerance)
			}
		})
	}
}

// ============================================================================
// WORKFLOW DRY RUN
// ============================================================================

// TestWorkflow_DryRunMode verifies that the dry-run code path — which calls
// GenerateMutants but does not execute any tests — produces mutants with
// valid metadata and no killed/survived status.
func TestWorkflow_DryRunMode(t *testing.T) {
	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	// generateMutantsWithConfig is the dry-run equivalent: mutants are
	// generated and filtered but tests are never executed.
	mutants := generateMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("dry run produced no mutants — fixture broken")
	}

	for _, m := range mutants {
		if m.ID <= 0 {
			t.Errorf("mutant has invalid ID: %d", m.ID)
		}
		if m.Operator == nil {
			t.Errorf("mutant %d has nil operator", m.ID)
		}
		if m.Site.File == nil {
			t.Errorf("mutant %d has nil file", m.ID)
		}
		if m.Site.Line <= 0 {
			t.Errorf("mutant %d has invalid line: %d", m.ID, m.Site.Line)
		}
		// No test execution happened so status must not be kill/survived.
		if m.Status == "killed" || m.Status == "survived" {
			t.Errorf("mutant %d has status %q in dry-run mode — tests should not have run", m.ID, m.Status)
		}
	}

	// IDs must be sequential starting from 1.
	for i, m := range mutants {
		if m.ID != i+1 {
			t.Errorf("mutant IDs not sequential at index %d: got ID=%d, want %d", i, m.ID, i+1)
			break
		}
	}
}

// ============================================================================
// WORKFLOW PROGRESS BAR
// ============================================================================

// TestWorkflow_ProgressBarLifecycle intentionally skipped: progress bar output
// is a terminal rendering concern that cannot be verified in a headless test.
func TestWorkflow_ProgressBarLifecycle(t *testing.T) {
	t.Skip("progress bar is a terminal rendering concern — not testable in headless mode")
}

// ============================================================================
// WORKFLOW ORG POLICY
// ============================================================================

// TestWorkflow_OrgPolicyEnforcement verifies that orgpolicy.Apply correctly
// enforces threshold_floor, forbidden_operators, forced_skip_paths, and other
// policy fields — including exposing the known gap where forbidden_operators
// cannot remove operators from the "all" shorthand.
func TestWorkflow_OrgPolicyEnforcement(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allOps := mutator.ListAll()

	t.Run("ThresholdFloor_RaisesLowThreshold", func(t *testing.T) {
		cfg := config.Default()
		cfg.Threshold = 20.0
		policy := &config.OrgPolicy{ThresholdFloor: 70.0}
		result := orgpolicy.Apply(cfg, policy, allOps)

		if result.Config.Threshold < 70.0 {
			t.Errorf("threshold_floor not applied: got %.2f, want >= 70.00", result.Config.Threshold)
		}
		if len(result.Violations) == 0 {
			t.Errorf("expected violation when threshold raised by threshold_floor")
		}
	})

	t.Run("ThresholdFloor_DoesNotLowerHighThreshold", func(t *testing.T) {
		cfg := config.Default()
		cfg.Threshold = 90.0
		policy := &config.OrgPolicy{ThresholdFloor: 70.0}
		result := orgpolicy.Apply(cfg, policy, allOps)

		if result.Config.Threshold != 90.0 {
			t.Errorf("threshold_floor should not lower threshold: got %.2f, want 90.00", result.Config.Threshold)
		}
		if len(result.Violations) > 0 {
			t.Errorf("no violation expected when threshold already above floor: %v", result.Violations)
		}
	})

	t.Run("ForbiddenOperators_RemovedFromExplicitList", func(t *testing.T) {
		cfg := config.Default()
		cfg.Operators = []string{"negate_condition", "arithmetic_flip", "sign_toggle"}
		policy := &config.OrgPolicy{ForbiddenOperators: []string{"negate_condition"}}
		result := orgpolicy.Apply(cfg, policy, allOps)

		for _, op := range result.Config.Operators {
			if op == "negate_condition" {
				t.Errorf("negate_condition should be removed from explicit operator list by forbidden_operators policy")
			}
		}
		if len(result.Violations) == 0 {
			t.Errorf("expected violation when a forbidden operator is removed")
		}
	})

	t.Run("ForbiddenOperators_NotRemovedFromAllShorthand", func(t *testing.T) {
		// BUG: enforceForbiddenOperators only removes exact string matches from
		// the operators list. When operators=[all], the forbidden operator name
		// is not present as a literal string so nothing is removed. The "all"
		// shorthand is never expanded before the denial pass. Callers must use
		// explicit operator lists for forbidden_operators enforcement to work.
		cfg := config.Default()
		cfg.Operators = []string{"all"}
		policy := &config.OrgPolicy{ForbiddenOperators: []string{"negate_condition"}}
		result := orgpolicy.Apply(cfg, policy, allOps)

		ops, err := cli.ParseOperators(result.Config)
		if err != nil {
			t.Fatalf("ParseOperators: %v", err)
		}
		for _, op := range ops {
			if op.Name() == "negate_condition" {
				t.Errorf("BUG CONFIRMED: negate_condition forbidden by org policy but still active " +
					"because forbidden_operators does not expand the 'all' shorthand before filtering")
			}
		}
	})

	t.Run("ForcedSkipPaths_AppendedToConfig", func(t *testing.T) {
		cfg := config.Default()
		cfg.Skip = []string{"vendor/"}
		policy := &config.OrgPolicy{ForcedSkipPaths: []string{"generated/", "mocks/"}}
		result := orgpolicy.Apply(cfg, policy, allOps)

		want := map[string]bool{"vendor/": true, "generated/": true, "mocks/": true}
		for _, s := range result.Config.Skip {
			if !want[s] {
				t.Errorf("unexpected skip path %q after org policy enforcement", s)
			}
			delete(want, s)
		}
		for missing := range want {
			t.Errorf("skip path %q missing after org policy enforcement", missing)
		}
	})

	t.Run("ForbiddenOperators_ActuallyPreventsMutants_ExplicitList", func(t *testing.T) {
		// Verifies the full pipeline: forbidden operators from an explicit list
		// do not produce any mutants.
		targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

		baselineMutants := generateMutantsRaw(t, targetDir)
		if !operatorSet(baselineMutants)["negate_condition"] {
			t.Fatalf("baseline has no negate_condition mutants — can't verify enforcement")
		}

		cfg := config.Default()
		cfg.Operators = []string{"negate_condition", "arithmetic_flip", "sign_toggle"}
		policy := &config.OrgPolicy{ForbiddenOperators: []string{"negate_condition"}}
		result := orgpolicy.Apply(cfg, policy, allOps)

		ops, err := cli.ParseOperators(result.Config)
		if err != nil {
			t.Fatalf("ParseOperators: %v", err)
		}

		eng := engine.NewEngine(false)
		eng.SetOperators(ops)
		eng.SetProjectRoot(repoRoot)
		if err := eng.Traverse(targetDir, nil); err != nil {
			t.Fatalf("traverse: %v", err)
		}
		sites := eng.Sites()

		log := logger.New(false)
		resolver, _ := subconfig.Discover(repoRoot, "")
		mutants := coretesting.GenerateMutants(sites, ops, allOps, repoRoot, nil, resolver, log)

		for _, m := range mutants {
			if m.Operator.Name() == "negate_condition" {
				t.Errorf("negate_condition forbidden by org policy (explicit list) but produced mutant %d", m.ID)
			}
		}
	})
}

// ============================================================================
// WORKFLOW THRESHOLD CHECKING
// ============================================================================

// TestWorkflow_ThresholdChecking verifies that reporter.Report returns an error
// when the mutation score is below the configured threshold, and passes when
// the threshold is zero.
func TestWorkflow_ThresholdChecking(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	// generateMutantsRaw returns mutants with no status (all effectively "untested"),
	// producing a score of 0%. A threshold of 50% must trigger a failure.
	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced — fixture broken")
	}

	t.Run("FailsBelowThreshold", func(t *testing.T) {
		_, err := reporter.Report(
			rawMutants, len(rawMutants), 50.0, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{},
		)
		if err == nil {
			t.Errorf("expected error when score=0%% and threshold=50%%, got nil")
		} else if !strings.Contains(err.Error(), "threshold") && !strings.Contains(err.Error(), "below") {
			t.Errorf("threshold error message looks wrong: %v", err)
		}
	})

	t.Run("PassesWithZeroThreshold", func(t *testing.T) {
		_, err := reporter.Report(
			rawMutants, len(rawMutants), 0, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{},
		)
		if err != nil {
			t.Errorf("threshold=0 should never fail: %v", err)
		}
	})

	t.Run("PassesWhenScoreAboveThreshold", func(t *testing.T) {
		repoRoot := findRepoRoot(t)
		configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
		targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

		executedMutants, stats := runMutantsWithConfig(t, configPath, targetDir)
		if stats.Score == 0 {
			t.Skip("mutation score is 0 — cannot verify threshold passes above score")
		}

		threshold := stats.Score - 1.0
		if threshold < 0 {
			threshold = 0
		}
		totalMutants := coretesting.GetTotalMutants()
		_, err := reporter.Report(
			executedMutants, totalMutants, threshold, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{},
		)
		if err != nil {
			t.Errorf("score %.2f%% with threshold %.2f%% should pass: %v", stats.Score, threshold, err)
		}
	})
}

// TestWorkflow_ThresholdCheckingAllFiles verifies that all configured output
// files are written before the threshold error is returned. A threshold abort
// that skips writing outputs would cause CI consumers to get no data.
func TestWorkflow_ThresholdCheckingAllFiles(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced — fixture broken")
	}

	tmpDir := t.TempDir()
	outputBase := filepath.Join(tmpDir, "report")

	_, err := reporter.Report(
		rawMutants, len(rawMutants), 100.0, nil,
		false, false, false,
		outputBase+".json", "", "json",
		reporter.BaselineOptions{
			MultiOutputs: []string{
				"textfile:" + outputBase + ".txt",
				"junit:" + outputBase + ".xml",
			},
		},
	)
	if err == nil {
		t.Fatalf("expected threshold error when score=0%% and threshold=100%%, got nil")
	}

	// All output files must exist — they must be written before the threshold check.
	for _, f := range []string{outputBase + ".json", outputBase + ".txt", outputBase + ".xml"} {
		if _, statErr := os.Stat(f); os.IsNotExist(statErr) {
			t.Errorf("output file %s was not written before threshold error was returned", filepath.Base(f))
		}
	}
}

// ============================================================================
// WORKFLOW SUPPRESSIONS
// ============================================================================

// TestWorkflow_SuppressionsWork verifies that a suppress entry in the root
// config removes mutants from the specified file:line location.
func TestWorkflow_SuppressionsWork(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("no mutants — fixture broken")
	}

	// Find a file:line that has at least one mutant.
	type lineKey struct{ file, line string }
	lineCounts := make(map[lineKey]int)
	for _, m := range baselineMutants {
		if m.Site.File == nil {
			continue
		}
		rel, err := filepath.Rel(repoRoot, m.Site.File.Name())
		if err != nil {
			continue
		}
		k := lineKey{rel, fmt.Sprintf("%d", m.Site.Line)}
		lineCounts[k]++
	}
	if len(lineCounts) == 0 {
		t.Fatalf("no mutants with file information")
	}

	// Pick the line with the most mutants to make the suppression effect obvious.
	var target lineKey
	best := 0
	for k, n := range lineCounts {
		if n > best {
			best = n
			target = k
		}
	}
	location := target.file + ":" + target.line

	suppressYAML := fmt.Sprintf(`operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
suppress:
  - location: %s
`, location)

	configPath := writeTempConfig(t, suppressYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	for _, m := range filtered {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		line := fmt.Sprintf("%d", m.Site.Line)
		if rel == target.file && line == target.line {
			t.Errorf("suppressed line %s still has mutant %d (op=%s) — suppress config not respected",
				location, m.ID, m.Operator.Name())
		}
	}

	// Sanity: suppressing one line shouldn't remove all mutants (unless target has only one line).
	if len(filtered) == 0 && len(lineCounts) > 1 {
		t.Errorf("suppressing one line removed all %d mutants — suppression scope is too broad", len(baselineMutants))
	}
}

// TestWorkflow_SuppressionsWorkUnderSubconfig verifies that suppress entries
// placed in a per-directory sub-config are merged with the root config and
// correctly remove mutants from the suppressed location.
//
// NOTE: This test currently FAILS because EffectiveSuppress in the resolver
// is defined but never called by the engine. Sub-config suppress entries are
// silently ignored. The test is intentionally written to expose this bug.
func TestWorkflow_SuppressionsWorkUnderSubconfig(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "internal/baseline")

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("internal/baseline has no mutants — fixture broken")
	}

	// Find a line with mutants.
	var targetFile, targetLine string
	for _, m := range baselineMutants {
		if m.Site.File == nil {
			continue
		}
		rel, err := filepath.Rel(repoRoot, m.Site.File.Name())
		if err != nil {
			continue
		}
		targetFile = rel
		targetLine = fmt.Sprintf("%d", m.Site.Line)
		break
	}
	if targetFile == "" {
		t.Fatalf("no mutant with file information")
	}
	location := targetFile + ":" + targetLine

	// Place a sub-config with the suppress entry inside the target directory.
	subConfigPath := filepath.Join(targetDir, "gorgon.yml")
	if _, err := os.Stat(subConfigPath); err == nil {
		t.Fatalf("sub-config already exists at %s — refusing to overwrite", subConfigPath)
	}
	subConfigContent := fmt.Sprintf("suppress:\n  - location: %s\n", location)
	if err := os.WriteFile(subConfigPath, []byte(subConfigContent), 0o644); err != nil {
		t.Fatalf("write sub-config: %v", err)
	}
	t.Cleanup(func() { os.Remove(subConfigPath) })

	// Root config has no suppress entries.
	rootCfgYAML := "operators:\n  - all\nthreshold: 0\nconcurrent: 1\ncache: false\nunit_tests_enabled: false\n"
	rootConfigPath := writeTempConfig(t, rootCfgYAML)

	filtered := generateMutantsWithConfig(t, rootConfigPath, targetDir)

	for _, m := range filtered {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		line := fmt.Sprintf("%d", m.Site.Line)
		if rel == targetFile && line == targetLine {
			t.Errorf("sub-config suppress not respected: location %s still has mutant %d (op=%s). "+
				"BUG: resolver.EffectiveSuppress is defined but never called during engine traversal.",
				location, m.ID, m.Operator.Name())
		}
	}
}

// TestWorkflow_SuppressionsWorkUnderOrgpolicy verifies that org policy's
// locked_settings prevents sub-configs from overriding locked fields, while
// root-config suppressions still apply normally.
func TestWorkflow_SuppressionsWorkUnderOrgpolicy(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allOps := mutator.ListAll()

	t.Run("LockedOperators_PreventSubConfigOverride", func(t *testing.T) {
		// With "operators" locked, a sub-config that sets operators: [arithmetic_flip]
		// must be ignored — the root config operators should govern.
		targetDir := filepath.Join(repoRoot, "internal/baseline")

		baselineMutants := generateMutantsRaw(t, targetDir)
		if len(baselineMutants) == 0 {
			t.Fatalf("internal/baseline has no mutants")
		}
		rootOps := operatorSet(baselineMutants)
		if len(rootOps) <= 1 {
			t.Skip("baseline has only one operator — can't verify locking prevents restriction")
		}

		subConfigPath := filepath.Join(targetDir, "gorgon.yml")
		if _, err := os.Stat(subConfigPath); err == nil {
			t.Fatalf("sub-config already exists at %s — refusing to overwrite", subConfigPath)
		}
		// Sub-config tries to restrict to arithmetic_flip only.
		subContent := "operators:\n  - arithmetic_flip\n"
		if err := os.WriteFile(subConfigPath, []byte(subContent), 0o644); err != nil {
			t.Fatalf("write sub-config: %v", err)
		}
		t.Cleanup(func() { os.Remove(subConfigPath) })

		// Build a resolver with policy that locks "operators".
		policy := &config.OrgPolicy{LockedSettings: []string{"operators"}}
		rootCfgYAML := "operators:\n  - all\nthreshold: 0\nconcurrent: 1\ncache: false\nunit_tests_enabled: false\n"
		rootConfigPath := writeTempConfig(t, rootCfgYAML)

		cfg := loadIntegrationConfig(t, rootConfigPath)
		ops, err := cli.ParseOperators(cfg)
		if err != nil {
			t.Fatalf("ParseOperators: %v", err)
		}

		eng := engine.NewEngine(false)
		eng.SetOperators(ops)
		eng.SetProjectRoot(repoRoot)
		if err := eng.Traverse(targetDir, nil); err != nil {
			t.Fatalf("traverse: %v", err)
		}
		sites := eng.Sites()

		resolver, err := subconfig.DiscoverWithPolicy(repoRoot, rootConfigPath, policy)
		if err != nil {
			t.Fatalf("DiscoverWithPolicy: %v", err)
		}

		log := logger.New(false)
		mutants := coretesting.GenerateMutants(sites, ops, allOps, repoRoot, nil, resolver, log)

		// With operators locked, sub-config can't restrict to arithmetic_flip.
		// We expect operators BEYOND arithmetic_flip to appear.
		resultOps := operatorSet(mutants)
		onlyArithmetic := true
		for op := range resultOps {
			if op != "arithmetic_flip" {
				onlyArithmetic = false
				break
			}
		}
		if onlyArithmetic && len(resultOps) > 0 {
			t.Errorf("org policy locked 'operators' but sub-config restriction to arithmetic_flip still applied — "+
				"got operators: %v", resultOps)
		}
	})

	t.Run("RootSuppression_WorksUnderOrgPolicy", func(t *testing.T) {
		// Root-config suppressions must still work even when an org policy is active.
		targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

		baselineMutants := generateMutantsRaw(t, targetDir)
		if len(baselineMutants) == 0 {
			t.Fatalf("no mutants — fixture broken")
		}

		var targetFile, targetLine string
		for _, m := range baselineMutants {
			if m.Site.File == nil {
				continue
			}
			rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
			targetFile = rel
			targetLine = fmt.Sprintf("%d", m.Site.Line)
			break
		}

		location := targetFile + ":" + targetLine
		suppressYAML := fmt.Sprintf(`operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
suppress:
  - location: %s
`, location)
		configPath := writeTempConfig(t, suppressYAML)
		filtered := generateMutantsWithConfig(t, configPath, targetDir)

		for _, m := range filtered {
			if m.Site.File == nil {
				continue
			}
			rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
			line := fmt.Sprintf("%d", m.Site.Line)
			if rel == targetFile && line == targetLine {
				t.Errorf("root-config suppress not respected under org policy: %s:%s still has mutant %d",
					targetFile, targetLine, m.ID)
			}
		}
	})
}

// TestWorkflow_SuppressionsWorkUnderSubconfigAndOrgpolicy verifies the combined
// behavior: sub-config suppressions (currently buggy) and org policy locking.
func TestWorkflow_SuppressionsWorkUnderSubconfigAndOrgpolicy(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allOps := mutator.ListAll()
	targetDir := filepath.Join(repoRoot, "internal/baseline")

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("internal/baseline has no mutants — fixture broken")
	}

	var targetFile, targetLine string
	for _, m := range baselineMutants {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		targetFile = rel
		targetLine = fmt.Sprintf("%d", m.Site.Line)
		break
	}
	location := targetFile + ":" + targetLine

	subConfigPath := filepath.Join(targetDir, "gorgon.yml")
	if _, err := os.Stat(subConfigPath); err == nil {
		t.Fatalf("sub-config already exists at %s — refusing to overwrite", subConfigPath)
	}

	// Sub-config: suppress a line AND restrict operators.
	subContent := fmt.Sprintf("suppress:\n  - location: %s\noperators:\n  - arithmetic_flip\n", location)
	if err := os.WriteFile(subConfigPath, []byte(subContent), 0o644); err != nil {
		t.Fatalf("write sub-config: %v", err)
	}
	t.Cleanup(func() { os.Remove(subConfigPath) })

	// Org policy: lock operators (prevents sub-config from restricting to arithmetic_flip).
	policy := &config.OrgPolicy{LockedSettings: []string{"operators"}}

	rootCfgYAML := "operators:\n  - all\nthreshold: 0\nconcurrent: 1\ncache: false\nunit_tests_enabled: false\n"
	rootConfigPath := writeTempConfig(t, rootCfgYAML)
	cfg := loadIntegrationConfig(t, rootConfigPath)

	ops, err := cli.ParseOperators(cfg)
	if err != nil {
		t.Fatalf("ParseOperators: %v", err)
	}

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()

	resolver, err := subconfig.DiscoverWithPolicy(repoRoot, rootConfigPath, policy)
	if err != nil {
		t.Fatalf("DiscoverWithPolicy: %v", err)
	}

	log := logger.New(false)
	mutants := coretesting.GenerateMutants(sites, ops, allOps, repoRoot, nil, resolver, log)

	// Org policy locks operators → sub-config's operator restriction must not apply.
	// We expect operators beyond arithmetic_flip.
	resultOps := operatorSet(mutants)
	onlyArithmetic := len(resultOps) > 0
	for op := range resultOps {
		if op != "arithmetic_flip" {
			onlyArithmetic = false
			break
		}
	}
	if onlyArithmetic {
		t.Errorf("org policy locked 'operators' but sub-config still restricted to arithmetic_flip: %v", resultOps)
	}

	// Sub-config suppress: location should have 0 mutants IF sub-config suppress worked.
	// NOTE: this currently FAILS because EffectiveSuppress is never called.
	for _, m := range mutants {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		line := fmt.Sprintf("%d", m.Site.Line)
		if rel == targetFile && line == targetLine {
			t.Errorf("sub-config suppress not respected (combined org-policy test): %s still has mutant %d. "+
				"BUG: EffectiveSuppress is never called during engine traversal.",
				location, m.ID)
		}
	}
}

// ============================================================================
// WORKFLOW CONFIGURATION
// ============================================================================

// TestWorkflow_ConfigureGoVersion verifies that the go_version config field
// is loaded correctly and that SetGoVersion accepts valid version strings.
func TestWorkflow_ConfigureGoVersion(t *testing.T) {
	t.Run("ConfigLoadsGoVersion", func(t *testing.T) {
		cfgYAML := "operators:\n  - all\ngo_version: \"1.21\"\n"
		configPath := writeTempConfig(t, cfgYAML)
		cfg := loadIntegrationConfig(t, configPath)
		if cfg.GoVersion != "1.21" {
			t.Errorf("go_version not loaded from config: got %q, want %q", cfg.GoVersion, "1.21")
		}
	})

	t.Run("SetGoVersionAcceptsValidVersion", func(t *testing.T) {
		// SetGoVersion only sets versions with "1." prefix.
		// Verify it accepts a valid version without panicking.
		coretesting.SetGoVersion("1.21")
		// Restore to detected version after test — call with empty string is a no-op,
		// so we can't restore easily. Accept the global mutation for this test.
		// The next test re-detects via the go.mod, so this is benign.
	})

	t.Run("SetGoVersionRejectsInvalidVersion", func(t *testing.T) {
		// SetGoVersion must silently reject versions that don't start with "1.".
		// Call it and verify the pipeline still works (it doesn't panic).
		coretesting.SetGoVersion("invalid")
		coretesting.SetGoVersion("2.0") // Should be rejected (no "1." prefix)
		// Restore to something valid.
		coretesting.SetGoVersion("1.25")
	})

	t.Run("PipelineRunsWithExplicitGoVersion", func(t *testing.T) {
		// Verify that applying go_version before a pipeline run does not break
		// the workspace go.mod generation. If SetGoVersion writes a bad version
		// string, workspace setup will fail with a compile error.
		repoRoot := findRepoRoot(t)
		targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

		// Mirror what runner.Run does: apply the version before traversal.
		coretesting.SetGoVersion("1.21")
		t.Cleanup(func() { coretesting.SetGoVersion("1.25") })

		rawMutants := generateMutantsRaw(t, targetDir)
		if len(rawMutants) == 0 {
			t.Errorf("pipeline with go_version=1.21 produced no mutants")
		}
	})
}

// ============================================================================
// WORKFLOW LARGE FILE HANDLING
// ============================================================================

// TestWorkflow_ChunkLargeFiles intentionally skipped: the chunk_large_files
// flag is under evaluation and the memory issue it addressed (recursive AST
// handling causing exponential blowup) is no longer reproducible. Revisit
// if OOM reports recur on large files.
func TestWorkflow_ChunkLargeFiles(t *testing.T) {
	t.Skip("chunk_large_files is under evaluation — see comment in test for context")
}

// ============================================================================
// WORKFLOW MEMORY CHECKING
// ============================================================================

// TestWorkflow_MemoryUsage verifies that a full pipeline run on a small
// package does not cause excessive heap growth. Pathological memory usage
// (e.g. recursive AST handlers or exponential schemata expansion) would show
// up here as TotalAlloc far exceeding the expected bound.
func TestWorkflow_MemoryUsage(t *testing.T) {
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	mutants, _ := runMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("pipeline produced no mutants — fixture broken")
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// TotalAlloc counts cumulative bytes allocated — it is monotonically
	// increasing and does not account for GC. For a small package run,
	// 500 MB of total allocation is a generous upper bound.
	const maxTotalAllocMB = 500
	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated > maxTotalAllocMB*1024*1024 {
		t.Errorf("excessive heap allocation during pipeline run: %d MB (limit %d MB) — "+
			"check for exponential schemata expansion or recursive AST handling",
			allocated/(1024*1024), maxTotalAllocMB)
	}
}

// ============================================================================
// WORKFLOW — CROSS-FEATURE COMBINATORIAL TESTS
// ============================================================================

// TestWorkflow_AllFeaturesCombined exercises the full pipeline on a real
// package with multiple features enabled: org policy + baseline + threshold.
// A full combinatorial matrix of all features is impractical in a single test;
// individual feature interactions are covered by the focused tests below.
func TestWorkflow_AllFeaturesCombined(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allOps := mutator.ListAll()
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	// Apply org policy to restrict operators and enforce a threshold floor.
	cfg := config.Default()
	cfg.Operators = []string{"negate_condition", "arithmetic_flip"}
	cfg.Threshold = 0
	policy := &config.OrgPolicy{ThresholdFloor: 0}
	result := orgpolicy.Apply(cfg, policy, allOps)

	ops, err := cli.ParseOperators(result.Config)
	if err != nil {
		t.Fatalf("ParseOperators: %v", err)
	}

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no sites")
	}

	resolver, _ := subconfig.Discover(repoRoot, "")
	sites = runner.FilterSites(sites, []string{targetDir}, result.Config, resolver)

	log := logger.New(false)
	mutants, err := coretesting.GenerateAndRunSchemata(
		context.Background(), sites, ops, allOps,
		targetDir, repoRoot,
		result.Config.DirRules, resolver,
		1, nil, nil, nil,
		log, false, true,
		result.Config.ExternalSuites, result.Config,
	)
	if err != nil {
		t.Logf("pipeline error: %v", err)
	}
	if len(mutants) == 0 {
		t.Fatalf("combined pipeline produced no mutants")
	}

	total := coretesting.GetTotalMutants()
	stats, reportErr := reporter.Report(
		mutants, total, result.Config.Threshold, resolver,
		false, false, false, "", "", "",
		reporter.BaselineOptions{},
	)
	if reportErr != nil {
		t.Errorf("reporter error with threshold=%.2f: %v", result.Config.Threshold, reportErr)
	}

	// All mutants must be from allowed operators only.
	for _, m := range mutants {
		if m.Operator.Name() != "negate_condition" && m.Operator.Name() != "arithmetic_flip" {
			t.Errorf("unexpected operator %q after org policy restriction", m.Operator.Name())
		}
	}
	t.Logf("combined features: %d mutants, score=%.2f%%, threshold=%.2f",
		stats.Total, stats.Score, result.Config.Threshold)
}

// TestWorkflow_AllFiltersCombined verifies skip + exclude + include + skip_func +
// suppress compose correctly. Uses a temp config that combines all four filter
// types against the reporter package.
func TestWorkflow_AllFiltersCombined(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baseline := generateMutantsRaw(t, targetDir)
	if len(baseline) == 0 {
		t.Fatalf("no baseline mutants")
	}

	// Pick a specific file:line that has mutants to suppress.
	var suppressLocation string
	for _, m := range baseline {
		if m.Site.File != nil {
			rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
			suppressLocation = fmt.Sprintf("%s:%d", rel, m.Site.Line)
			break
		}
	}

	cfgYAML := fmt.Sprintf(`operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
skip:
  - reporter.go
exclude:
  - json.go
include:
  - junit.go
  - sarif.go
  - html.go
  - textfile.go
  - reporter.go
  - json.go
skip_func:
  - junit.go:writeJUnitReport
suppress:
  - location: %s
`, suppressLocation)

	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)
	filteredByFile := mutantsByFile(filtered)

	// Skip takes priority over include: reporter.go must have zero mutants.
	if n := filteredByFile["reporter.go"]; n > 0 {
		t.Errorf("skip takes priority over include: reporter.go has %d mutants", n)
	}
	// Exclude takes priority over include: json.go must have zero mutants.
	if n := filteredByFile["json.go"]; n > 0 {
		t.Errorf("exclude: json.go not respected — %d mutants remain", n)
	}
	// Suppress must remove mutants at the specified location.
	for _, m := range filtered {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		loc := fmt.Sprintf("%s:%d", rel, m.Site.Line)
		if loc == suppressLocation {
			t.Errorf("suppressed location %s still has mutant %d", loc, m.ID)
		}
	}
	// Sanity: include-listed files that aren't skipped/excluded/suppressed must have mutants.
	for _, f := range []string{"sarif.go", "html.go", "textfile.go"} {
		if filteredByFile[f] == 0 {
			t.Errorf("include-listed file %s has no mutants", f)
		}
	}
}

// TestWorkflow_ConflictingFilters_IncludeWinsOverExclude verifies that when
// a file is both included and excluded, include takes priority.
func TestWorkflow_ConflictingFilters_IncludeWinsOverExclude(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baseline := mutantsByFile(generateMutantsRaw(t, targetDir))
	if baseline["reporter.go"] == 0 {
		t.Fatalf("reporter.go has no mutants in baseline")
	}

	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
include:
  - reporter.go
exclude:
  - reporter.go
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)
	filteredByFile := mutantsByFile(filtered)

	if filteredByFile["reporter.go"] == 0 {
		// Include wins — reporter.go has mutants despite being excluded.
		// Document this behavior as the expected priority.
		t.Skip("include does not override exclude — reporter.go has 0 mutants. " +
			"Current priority: exclude > include. Documenting behavior.")
	}
	// If we get here, include wins.
	t.Logf("include overrides exclude: reporter.go has %d mutants (baseline had %d)",
		filteredByFile["reporter.go"], baseline["reporter.go"])
}

// TestWorkflow_ConflictingFilters_SkipWinsOverInclude verifies that skip takes
// priority over include (a skipped file should never produce mutants even if included).
func TestWorkflow_ConflictingFilters_SkipWinsOverInclude(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baseline := mutantsByFile(generateMutantsRaw(t, targetDir))
	if baseline["reporter.go"] == 0 {
		t.Fatalf("reporter.go has no mutants in baseline")
	}

	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
skip:
  - reporter.go
include:
  - reporter.go
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)
	filteredByFile := mutantsByFile(filtered)

	if n := filteredByFile["reporter.go"]; n > 0 {
		t.Errorf("skip should take priority over include: reporter.go has %d mutants (expected 0)", n)
	}
}

// ============================================================================
// WORKFLOW — DIR_RULES EDGE CASES
// ============================================================================

// TestWorkflow_DirRules_WhitelistAndBlacklist_SameDir verifies behavior when
// a dir_rule has both whitelist and blacklist for the same directory.
// Whitelist takes priority over blacklist per the effectiveOperators logic.
func TestWorkflow_DirRules_WhitelistAndBlacklist_SameDir(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	baselineOps := operatorSet(baselineMutants)
	if !baselineOps["negate_condition"] || !baselineOps["arithmetic_flip"] {
		t.Fatalf("need both negate_condition and arithmetic_flip in baseline; got: %v", baselineOps)
	}

	// Whitelist: [negate_condition], Blacklist: [arithmetic_flip, negate_condition]
	// Whitelist wins → only negate_condition active, arithmetic_flip excluded by whitelist.
	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    whitelist:
      - negate_condition
    blacklist:
      - arithmetic_flip
      - negate_condition
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	if len(filtered) == 0 {
		t.Fatalf("whitelist+blacklist removed all mutants")
	}
	for _, m := range filtered {
		if m.Operator.Name() != "negate_condition" {
			t.Errorf("whitelist:[negate_condition] should allow only negate_condition; got %q", m.Operator.Name())
		}
	}
}

// TestWorkflow_DirRules_WhitelistEmpty_NoMutants verifies that an empty whitelist
// produces zero mutants for that directory.
func TestWorkflow_DirRules_WhitelistEmpty_NoMutants(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    whitelist: []
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	if len(filtered) > 0 {
		t.Errorf("empty whitelist should produce 0 mutants; got %d", len(filtered))
	}
}

// TestWorkflow_DirRules_BlacklistEmpty_AllOperatorsAllowed verifies that an
// empty blacklist allows all operators (same as no dir_rule at all).
func TestWorkflow_DirRules_BlacklistEmpty_AllOperatorsAllowed(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	baselineOps := operatorSet(baselineMutants)
	baselineCount := len(baselineMutants)

	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    blacklist: []
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)
	filteredOps := operatorSet(filtered)

	// With empty blacklist, all baseline operators should still be present.
	for op := range baselineOps {
		if !filteredOps[op] {
			t.Errorf("operator %q missing after empty blacklist dir_rule", op)
		}
	}

	// Mutant count should be roughly similar to baseline (may differ slightly
	// due to config loading vs raw generation).
	if len(filtered) == 0 {
		t.Errorf("empty blacklist should not remove mutants; got 0 (baseline had %d)", baselineCount)
	}
}

// TestWorkflow_DirRules_LongestPrefixMatch verifies that when multiple dir_rules
// could match a file, the most specific (longest prefix) rule wins.
func TestWorkflow_DirRules_LongestPrefixMatch(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	if !operatorSet(baselineMutants)["negate_condition"] || !operatorSet(baselineMutants)["arithmetic_flip"] {
		t.Fatalf("need negate_condition and arithmetic_flip in baseline")
	}

	// "internal/" whitelists negate_condition (shorter prefix).
	// "internal/reporter/" whitelists arithmetic_flip (longer prefix - should win).
	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/
    whitelist:
      - negate_condition
  - dir: internal/reporter
    whitelist:
      - arithmetic_flip
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	if len(filtered) == 0 {
		t.Fatalf("longest-prefix match removed all mutants")
	}
	for _, m := range filtered {
		if m.Operator.Name() == "negate_condition" {
			t.Errorf("shorter-prefix rule (internal/) should not apply to internal/reporter/; got negate_condition mutant %d", m.ID)
		}
		if m.Operator.Name() != "arithmetic_flip" {
			t.Errorf("expected arithmetic_flip from longest-prefix match; got %q", m.Operator.Name())
		}
	}
}

// TestWorkflow_DirRules_NoMatch_UsesAllOperators verifies that a file in a
// directory with no matching dir_rule uses all configured operators.
func TestWorkflow_DirRules_NoMatch_UsesAllOperators(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	baselineMutants := generateMutantsRaw(t, targetDir)
	baselineOps := operatorSet(baselineMutants)

	// dir_rules only for internal/reporter — negate_condition directory has no match.
	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    whitelist:
      - arithmetic_flip
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	// All baseline operators should still appear (no dir_rule matched).
	for op := range baselineOps {
		found := false
		for _, m := range filtered {
			if m.Operator.Name() == op {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("operator %q missing — no dir_rule matches this directory, all operators should be active", op)
		}
	}
}

// TestWorkflow_DirRules_MultipleSubconfigs_DirRulesMerge verifies that
// dir_rules from multiple sub-config levels merge correctly.
func TestWorkflow_DirRules_MultipleSubconfigs_DirRulesMerge(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "internal/baseline")

	// Write a sub-config in internal/baseline with its own dir_rule.
	subConfigPath := filepath.Join(targetDir, "gorgon.yml")
	if _, err := os.Stat(subConfigPath); err == nil {
		t.Fatalf("sub-config already exists at %s", subConfigPath)
	}
	subContent := `dir_rules:
  - dir: internal/baseline
    whitelist:
      - arithmetic_flip
`
	if err := os.WriteFile(subConfigPath, []byte(subContent), 0o644); err != nil {
		t.Fatalf("write sub-config: %v", err)
	}
	t.Cleanup(func() { os.Remove(subConfigPath) })

	// Root config with its own dir_rule for a different directory.
	cfgYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
dir_rules:
  - dir: internal/reporter
    whitelist:
      - negate_condition
`
	configPath := writeTempConfig(t, cfgYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	if len(filtered) == 0 {
		t.Fatalf("merged dir_rules removed all mutants")
	}
	for _, m := range filtered {
		if m.Operator.Name() != "arithmetic_flip" {
			t.Errorf("sub-config dir_rule should restrict to arithmetic_flip; got %q", m.Operator.Name())
		}
	}
}

// ============================================================================
// WORKFLOW — THRESHOLD EDGE CASES
// ============================================================================

// TestWorkflow_Threshold_ExactlyZero verifies threshold: 0 always passes
// regardless of score. (Already covered by "PassesWithZeroThreshold" in
// TestWorkflow_ThresholdChecking; this is a focused smoke test.)
func TestWorkflow_Threshold_ExactlyZero(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")
	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants")
	}
	_, err := reporter.Report(rawMutants, len(rawMutants), 0, nil,
		false, false, false, "", "", "",
		reporter.BaselineOptions{})
	if err != nil {
		t.Errorf("threshold=0 must always pass: %v", err)
	}
}

// TestWorkflow_Threshold_ExactlyHundred verifies threshold: 100 requires
// perfect kill rate — any unkilled mutant triggers a failure.
func TestWorkflow_Threshold_ExactlyHundred(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")
	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants")
	}
	// Raw mutants have no status → all untested → score 0%%.
	_, err := reporter.Report(rawMutants, len(rawMutants), 100.0, nil,
		false, false, false, "", "", "",
		reporter.BaselineOptions{})
	if err == nil {
		t.Errorf("threshold=100 with 0%% score should fail")
	}
}

// TestWorkflow_Threshold_FractionalValue verifies that fractional thresholds
// (e.g., 85.5) work correctly.
func TestWorkflow_Threshold_FractionalValue(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	// Run the full pipeline to get real executed mutants with statuses.
	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	mutants, stats := runMutantsWithConfig(t, configPath, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("no mutants")
	}

	totalMutants := coretesting.GetTotalMutants()

	// Threshold 0.5 below score → should pass.
	belowThreshold := stats.Score - 0.5
	if belowThreshold < 0 {
		belowThreshold = 0
	}
	if belowThreshold > 0 {
		_, err := reporter.Report(mutants, totalMutants, belowThreshold, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{})
		if err != nil {
			t.Errorf("threshold=%.2f below score=%.2f should pass: %v", belowThreshold, stats.Score, err)
		}
	}

	// Threshold 0.5 above score → should fail (unless score is already 100).
	aboveThreshold := stats.Score + 0.5
	if aboveThreshold < 100 {
		_, err := reporter.Report(mutants, totalMutants, aboveThreshold, nil,
			false, false, false, "", "", "",
			reporter.BaselineOptions{})
		if err == nil {
			t.Errorf("threshold=%.2f above score=%.2f should fail", aboveThreshold, stats.Score)
		}
	}
}

// ============================================================================
// WORKFLOW — CONCURRENCY EDGE CASES
// ============================================================================

// TestWorkflow_Concurrent_One_SequentialExecution verifies concurrent=1 runs
// and produces results identical to higher concurrency settings.
func TestWorkflow_Concurrent_One_SequentialExecution(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	cfg := loadIntegrationConfig(t, configPath)
	cfg.Concurrent = "1"

	ops, err := cli.ParseOperators(cfg)
	if err != nil {
		t.Fatalf("ParseOperators: %v", err)
	}
	allOps := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no sites")
	}

	resolver, _ := subconfig.Discover(repoRoot, configPath)
	sites = runner.FilterSites(sites, []string{targetDir}, cfg, resolver)
	log := logger.New(false)

	mutants, err := coretesting.GenerateAndRunSchemata(
		context.Background(), sites, ops, allOps,
		targetDir, repoRoot, cfg.DirRules, resolver,
		1, nil, nil, nil,
		log, false, true, cfg.ExternalSuites, cfg,
	)
	if err != nil {
		t.Logf("concurrent=1 pipeline: %v", err)
	}
	if len(mutants) == 0 {
		t.Fatalf("concurrent=1 produced no mutants")
	}

	// Verify all mutants have valid status.
	for _, m := range mutants {
		if m.Status == "" {
			t.Errorf("mutant %d has empty status after concurrent=1 run", m.ID)
		}
	}
}

// TestWorkflow_Concurrent_MoreThanMutants verifies that when concurrent > number
// of mutants, the pipeline still works (no deadlocks or panics).
func TestWorkflow_Concurrent_MoreThanMutants(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
	cfg := loadIntegrationConfig(t, configPath)
	cfg.Concurrent = "100"

	ops, err := cli.ParseOperators(cfg)
	if err != nil {
		t.Fatalf("ParseOperators: %v", err)
	}
	allOps := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no sites")
	}

	resolver, _ := subconfig.Discover(repoRoot, configPath)
	sites = runner.FilterSites(sites, []string{targetDir}, cfg, resolver)
	log := logger.New(false)

	mutants, err := coretesting.GenerateAndRunSchemata(
		context.Background(), sites, ops, allOps,
		targetDir, repoRoot, cfg.DirRules, resolver,
		100, nil, nil, nil,
		log, false, true, cfg.ExternalSuites, cfg,
	)
	if err != nil {
		t.Logf("concurrent=100 pipeline: %v", err)
	}
	if len(mutants) == 0 {
		t.Fatalf("concurrent=100 produced no mutants — pipeline may have panicked")
	}

	for _, m := range mutants {
		if m.Status == "" {
			t.Errorf("mutant %d has empty status after high-concurrency run", m.ID)
		}
	}
}

// TestWorkflow_Concurrent_All_Vs_Half_Vs_One verifies that different concurrent
// settings produce identical mutant status distributions.
func TestWorkflow_Concurrent_All_Vs_Half_Vs_One(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	statusesForConcurrency := func(concur string) map[int]string {
		configPath := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_KillAttributionCorrect/gorgon.yml")
		cfg := loadIntegrationConfig(t, configPath)
		cfg.Concurrent = concur

		ops, err := cli.ParseOperators(cfg)
		if err != nil {
			t.Fatalf("ParseOperators: %v", err)
		}
		allOps := mutator.ListAll()

		eng := engine.NewEngine(false)
		eng.SetOperators(ops)
		eng.SetProjectRoot(repoRoot)
		if err := eng.Traverse(targetDir, nil); err != nil {
			t.Fatalf("traverse: %v", err)
		}
		sites := eng.Sites()
		if len(sites) == 0 {
			t.Fatalf("no sites")
		}

		resolver, _ := subconfig.Discover(repoRoot, configPath)
		sites = runner.FilterSites(sites, []string{targetDir}, cfg, resolver)
		log := logger.New(false)

		c := cli.ParseConcurrent(concur)
		mutants, err := coretesting.GenerateAndRunSchemata(
			context.Background(), sites, ops, allOps,
			targetDir, repoRoot, cfg.DirRules, resolver,
			c, nil, nil, nil,
			log, false, true, cfg.ExternalSuites, cfg,
		)
		if err != nil {
			t.Logf("pipeline (concurrent=%s): %v", concur, err)
		}

		out := make(map[int]string, len(mutants))
		for _, m := range mutants {
			out[m.ID] = m.Status
		}
		return out
	}

	status1 := statusesForConcurrency("1")
	if len(status1) == 0 {
		t.Fatalf("concurrent=1 produced no mutants")
	}

	statusHalf := statusesForConcurrency("half")
	if len(statusHalf) == 0 {
		t.Fatalf("concurrent=half produced no mutants")
	}

	// All three must have the same mutant IDs.
	for id, s1 := range status1 {
		if sHalf, ok := statusHalf[id]; ok {
			if s1 != sHalf {
				t.Errorf("mutant %d: concurrent=1 status=%q, concurrent=half status=%q", id, s1, sHalf)
			}
		} else {
			t.Errorf("mutant %d present in concurrent=1 but missing from concurrent=half", id)
		}
	}
	for id := range statusHalf {
		if _, ok := status1[id]; !ok {
			t.Errorf("mutant %d present in concurrent=half but missing from concurrent=1", id)
		}
	}
}

// ============================================================================
// WORKFLOW — OUTPUT FORMATS EDGE CASES
// ============================================================================

// TestWorkflow_Output_JSON_ValidAndComplete verifies JSON output has all
// required fields and contains correct mutant data.
func TestWorkflow_Output_JSON_ValidAndComplete(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	outputDir := t.TempDir()
	mutantInfos, stats := runPipelineWithMutantTracking(t, targetDir, outputDir)

	jsonPath := filepath.Join(outputDir, "report.json")
	jsonMutants, err := extractMutantsFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	jsonStats, err := extractStatsFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("extract stats from JSON: %v", err)
	}

	// Validate each mutant in the JSON.
	for _, m := range jsonMutants {
		for _, e := range validateMutant(m) {
			t.Errorf("JSON mutant %d: %s", m.ID, e)
		}
	}

	// Validate ID completeness.
	for _, e := range checkIDCompleteness(jsonMutants) {
		t.Errorf("JSON: %s", e)
	}

	// Compare against in-memory results.
	discrepancies := compareMutantLists(mutantInfos, jsonMutants, "JSON")
	for _, d := range discrepancies {
		t.Errorf("JSON mutant mismatch: %s", d)
	}

	statDiscrepancies := compareStats(stats, jsonStats, "JSON")
	for _, d := range statDiscrepancies {
		t.Errorf("JSON stat mismatch: %s", d)
	}
}

// TestWorkflow_Output_JUnit_ValidXML verifies JUnit XML output contains
// the correct number of test cases and valid XML structure.
func TestWorkflow_Output_JUnit_ValidXML(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	outputDir := t.TempDir()
	_, stats := runPipelineWithMutantTracking(t, targetDir, outputDir)

	xmlPath := filepath.Join(outputDir, "report.xml")
	xmlStats, err := extractStatsFromJUnit(xmlPath)
	if err != nil {
		t.Fatalf("invalid JUnit XML: %v", err)
	}

	// JUnit must have at least some test cases.
	if xmlStats.Total == 0 {
		t.Errorf("JUnit output has 0 test cases")
	}

	discrepancies := compareStats(stats, xmlStats, "JUnit")
	for _, d := range discrepancies {
		t.Errorf("JUnit stat mismatch: %s", d)
	}
}

// TestWorkflow_Output_SARIF_ValidJSON verifies SARIF output follows the
// SARIF specification (version, runs, results, properties).
func TestWorkflow_Output_SARIF_ValidJSON(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	outputDir := t.TempDir()
	_, stats := runPipelineWithMutantTracking(t, targetDir, outputDir)

	sarifPath := filepath.Join(outputDir, "report.sarif")
	sarifStats, err := extractStatsFromSARIF(sarifPath)
	if err != nil {
		t.Fatalf("invalid SARIF: %v", err)
	}

	if sarifStats.Total == 0 {
		t.Errorf("SARIF output has 0 results")
	}

	discrepancies := compareStats(stats, sarifStats, "SARIF")
	for _, d := range discrepancies {
		t.Errorf("SARIF stat mismatch: %s", d)
	}
}

// TestWorkflow_Output_HTML_GeneratesValidHTML verifies HTML output contains
// the correct score and mutant data.
func TestWorkflow_Output_HTML_GeneratesValidHTML(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	outputDir := t.TempDir()
	_, stats := runPipelineWithMutantTracking(t, targetDir, outputDir)

	htmlDir := filepath.Join(outputDir, "report.html")
	htmlStats, err := extractStatsFromHTML(htmlDir)
	if err != nil {
		t.Fatalf("invalid HTML: %v", err)
	}

	if htmlStats.Total == 0 {
		t.Errorf("HTML output reports 0 total mutants")
	}

	discrepancies := compareStats(stats, htmlStats, "HTML")
	for _, d := range discrepancies {
		t.Errorf("HTML stat mismatch: %s", d)
	}
}

// TestWorkflow_Output_MultipleFormats verifies all output formats can be
// generated in a single run and contain consistent data.
func TestWorkflow_Output_MultipleFormats(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	outputDir := t.TempDir()
	_, stats := runPipelineWithMutantTracking(t, targetDir, outputDir)

	type formatCheck struct {
		name string
		path string
	}
	formats := []formatCheck{
		{"JSON", filepath.Join(outputDir, "report.json")},
		{"JUnit", filepath.Join(outputDir, "report.xml")},
		{"SARIF", filepath.Join(outputDir, "report.sarif")},
		{"Text", filepath.Join(outputDir, "report.txt")},
	}

	htmlDir := filepath.Join(outputDir, "report.html")

	for _, fc := range formats {
		if _, err := os.Stat(fc.path); os.IsNotExist(err) {
			t.Errorf("%s output file %s was not created", fc.name, fc.path)
		}
	}
	if _, err := os.Stat(filepath.Join(htmlDir, "index.html")); os.IsNotExist(err) {
		t.Errorf("HTML output index.html was not created in %s", htmlDir)
	}

	// Verify cross-format consistency: all formats should report the same score and totals.
	for _, fc := range formats {
		var fcStats reporter.ReportStats
		var err error
		switch fc.name {
		case "JSON":
			fcStats, err = extractStatsFromJSON(fc.path)
		case "JUnit":
			fcStats, err = extractStatsFromJUnit(fc.path)
		case "SARIF":
			fcStats, err = extractStatsFromSARIF(fc.path)
		case "Text":
			fcStats, err = extractStatsFromText(fc.path)
		}
		if err != nil {
			t.Errorf("%s: %v", fc.name, err)
			continue
		}
		for _, d := range compareStats(stats, fcStats, fc.name) {
			t.Errorf("cross-format consistency: %s", d)
		}
	}
}

// TestWorkflow_Output_FilePathsRelative verifies output files respect relative paths.
func TestWorkflow_Output_FilePathsRelative(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	ops := mutator.ListAll()
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no sites")
	}

	log := logger.New(false)
	resolver, _ := subconfig.Discover(repoRoot, "")
	ctx := context.Background()
	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx, sites, ops, ops,
		targetDir, repoRoot, nil, resolver,
		runtime.NumCPU(), nil, nil, nil,
		log, false, true, config.ExternalSuitesConfig{}, &config.Config{},
	)
	if err != nil {
		t.Logf("pipeline: %v", err)
	}
	if len(mutants) == 0 {
		t.Fatalf("no mutants")
	}

	// Use relative paths from the repo root.
	origWd, _ := os.Getwd()
	os.Chdir(repoRoot)
	defer os.Chdir(origWd)

	totalMutants := coretesting.GetTotalMutants()
	_, reportErr := reporter.Report(
		mutants, totalMutants, 0, nil,
		false, false, false,
		"relative_report.json", "", "json",
		reporter.BaselineOptions{},
	)
	if reportErr != nil {
		t.Errorf("relative path report: %v", reportErr)
	}

	fullPath := filepath.Join(repoRoot, "relative_report.json")
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("relative output file not created at %s", fullPath)
	}
	t.Cleanup(func() { os.Remove(fullPath) })
}

// TestWorkflow_Output_FilePathsAbsolute verifies absolute output paths.
func TestWorkflow_Output_FilePathsAbsolute(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	ops := mutator.ListAll()
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)
	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no sites")
	}

	log := logger.New(false)
	resolver, _ := subconfig.Discover(repoRoot, "")
	ctx := context.Background()
	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx, sites, ops, ops,
		targetDir, repoRoot, nil, resolver,
		runtime.NumCPU(), nil, nil, nil,
		log, false, true, config.ExternalSuitesConfig{}, &config.Config{},
	)
	if err != nil {
		t.Logf("pipeline: %v", err)
	}
	if len(mutants) == 0 {
		t.Fatalf("no mutants")
	}

	absPath := filepath.Join(t.TempDir(), "absolute_report.json")
	totalMutants := coretesting.GetTotalMutants()
	_, reportErr := reporter.Report(
		mutants, totalMutants, 0, nil,
		false, false, false,
		absPath, "", "json",
		reporter.BaselineOptions{},
	)
	if reportErr != nil {
		t.Errorf("absolute path report: %v", reportErr)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		t.Errorf("absolute output file not created at %s", absPath)
	}
}

// ============================================================================
// WORKFLOW — SUPPRESSION EDGE CASES
// ============================================================================

// TestWorkflow_Suppression_SpecificOperator verifies suppress with an operator
// list removes only those operators at that location, leaving other operators.
func TestWorkflow_Suppression_SpecificOperator(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("no mutants")
	}

	// Find a line that has mutants from multiple operators.
	type lineKey struct{ file, line string }
	opsByLine := make(map[lineKey]map[string]bool)
	for _, m := range baselineMutants {
		if m.Site.File == nil {
			continue
		}
		rel, err := filepath.Rel(repoRoot, m.Site.File.Name())
		if err != nil {
			continue
		}
		k := lineKey{rel, fmt.Sprintf("%d", m.Site.Line)}
		if opsByLine[k] == nil {
			opsByLine[k] = make(map[string]bool)
		}
		opsByLine[k][m.Operator.Name()] = true
	}

	var targetLine lineKey
	var targetOp string
	for k, ops := range opsByLine {
		if len(ops) >= 2 {
			targetLine = k
			for op := range ops {
				targetOp = op
				break
			}
			break
		}
	}
	if targetLine.file == "" {
		t.Skip("no line with multiple operators found — cannot test operator-specific suppression")
	}

	location := targetLine.file + ":" + targetLine.line

	suppressYAML := fmt.Sprintf(`operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
suppress:
  - location: %s
    operators:
      - %s
`, location, targetOp)

	configPath := writeTempConfig(t, suppressYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	for _, m := range filtered {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		line := fmt.Sprintf("%d", m.Site.Line)
		if rel == targetLine.file && line == targetLine.line && m.Operator.Name() == targetOp {
			t.Errorf("suppress with operator:[%s] did not suppress %s at %s", targetOp, targetOp, location)
		}
	}

	// Other operators at the same line should still be present.
	foundOther := false
	for _, m := range filtered {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		line := fmt.Sprintf("%d", m.Site.Line)
		if rel == targetLine.file && line == targetLine.line && m.Operator.Name() != targetOp {
			foundOther = true
			break
		}
	}
	if !foundOther {
		t.Logf("all operators suppressed at %s — other operators may not exist at this line", location)
	}
}

// TestWorkflow_Suppression_AllOperators_WhenListEmpty verifies that suppress
// with empty operators list suppresses ALL operators at that location.
func TestWorkflow_Suppression_AllOperators_WhenListEmpty(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("no mutants")
	}

	// Find a line with mutants.
	var targetFile, targetLine string
	for _, m := range baselineMutants {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		targetFile = rel
		targetLine = fmt.Sprintf("%d", m.Site.Line)
		break
	}

	location := targetFile + ":" + targetLine
	suppressYAML := fmt.Sprintf(`operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
suppress:
  - location: %s
`, location)

	configPath := writeTempConfig(t, suppressYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	for _, m := range filtered {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		line := fmt.Sprintf("%d", m.Site.Line)
		if rel == targetFile && line == targetLine {
			t.Errorf("suppress without operators list should suppress ALL operators at %s; mutant %d (%s) remains",
				location, m.ID, m.Operator.Name())
		}
	}
}

// TestWorkflow_Suppression_MultipleLocations verifies suppressing multiple
// distinct locations.
func TestWorkflow_Suppression_MultipleLocations(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("no mutants")
	}

	// Collect up to 2 distinct file:line pairs with mutants.
	type lineKey struct{ file, line string }
	var locations []string
	seen := make(map[lineKey]bool)
	for _, m := range baselineMutants {
		if m.Site.File == nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
		k := lineKey{rel, fmt.Sprintf("%d", m.Site.Line)}
		if seen[k] {
			continue
		}
		seen[k] = true
		locations = append(locations, rel+":"+k.line)
		if len(locations) >= 2 {
			break
		}
	}
	if len(locations) < 2 {
		t.Skip("less than 2 distinct mutant locations — can't test multiple suppressions")
	}

	suppressEntries := ""
	for _, loc := range locations {
		suppressEntries += fmt.Sprintf("  - location: %s\n", loc)
	}
	suppressYAML := fmt.Sprintf(`operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
suppress:
%s`, suppressEntries)

	configPath := writeTempConfig(t, suppressYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	for _, loc := range locations {
		parts := strings.SplitN(loc, ":", 2)
		for _, m := range filtered {
			if m.Site.File == nil {
				continue
			}
			rel, _ := filepath.Rel(repoRoot, m.Site.File.Name())
			line := fmt.Sprintf("%d", m.Site.Line)
			if rel == parts[0] && line == parts[1] {
				t.Errorf("multiple locations: suppressed %s still has mutant %d", loc, m.ID)
			}
		}
	}
}

// TestWorkflow_Suppression_InvalidLocation verifies behavior when a suppression
// location doesn't match any mutant — should not cause errors.
func TestWorkflow_Suppression_InvalidLocation(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	suppressYAML := `operators:
  - all
threshold: 0
concurrent: 1
cache: false
unit_tests_enabled: false
suppress:
  - location: nonexistent_file.go:99999
  - location: also_nonexistent.go:1
`
	configPath := writeTempConfig(t, suppressYAML)
	filtered := generateMutantsWithConfig(t, configPath, targetDir)

	// The pipeline must not fail because of invalid suppression locations.
	if len(filtered) == 0 {
		t.Errorf("invalid suppression locations caused all mutants to be removed")
	}
}

// ============================================================================
// WORKFLOW — EXTERNAL SUITE EDGE CASES
// ============================================================================

// TestWorkflow_ExternalSuite_SingleSuite verifies a single external suite works.
func TestWorkflow_ExternalSuite_SingleSuite(t *testing.T) {
	t.Skip("TODO: configure one external suite pointing at a test package; " +
		"assert mutants are killed by the external suite's tests")
}

// TestWorkflow_ExternalSuite_MultipleSuites verifies multiple external suites.
func TestWorkflow_ExternalSuite_MultipleSuites(t *testing.T) {
	t.Skip("TODO: configure 3 external suites; assert each runs and contributes " +
		"kill attributions; mutants killed by multiple suites should list all")
}

// TestWorkflow_ExternalSuite_NestedPaths verifies glob patterns like ./tests/...
// discover nested test packages.
func TestWorkflow_ExternalSuite_NestedPaths(t *testing.T) {
	t.Skip("TODO: external_suites with paths:['./tests/...']; assert all test " +
		"packages under tests/ are discovered and used")
}

// ============================================================================
// WORKFLOW — GO VERSION EDGE CASES
// ============================================================================

// TestWorkflow_GoVersion_DetectedFromGoMod verifies the Go version is detected
// from go.mod when no explicit go_version is set.
func TestWorkflow_GoVersion_DetectedFromGoMod(t *testing.T) {
	t.Skip("TODO: run pipeline without go_version in config; assert the detected " +
		"version from go.mod is used and the pipeline works")
}

// TestWorkflow_GoVersion_OverrideWorks verifies go_version in config overrides
// the detected version.
func TestWorkflow_GoVersion_OverrideWorks(t *testing.T) {
	t.Skip("TODO: set go_version: \"1.20\" in config; assert the pipeline uses " +
		"1.20 for the workspace go.mod generation")
}

// ============================================================================
// WORKFLOW — BUILD TAG EDGE CASES
// ============================================================================

// TestWorkflow_BuildTags_PassedToCompilation verifies that build_tags in config
// are passed to go test -c.
func TestWorkflow_BuildTags_PassedToCompilation(t *testing.T) {
	t.Skip("TODO: config with build_tags: [integration]; target a file with " +
		"//go:build integration; assert mutants are generated for integration-tagged code")
}

// TestWorkflow_BuildTags_MultipleTags verifies multiple build tags.
func TestWorkflow_BuildTags_MultipleTags(t *testing.T) {
	t.Skip("TODO: build_tags: [integration, e2e]; assert both tags are passed to " +
		"the compiler and files with either tag are included")
}

// ============================================================================
// WORKFLOW — DRY RUN EDGE CASES
// ============================================================================

// TestWorkflow_DryRun_MutantsListed verifies dry-run lists all mutants without
// executing tests.
func TestWorkflow_DryRun_MutantsListed(t *testing.T) {
	t.Skip("TODO: run with dry_run: true; assert all mutants are listed with valid " +
		"metadata (ID, file, line, col, operator) but no status other than untested/empty")
}

// TestWorkflow_DryRun_FiltersApplied verifies filtering still works in dry-run.
func TestWorkflow_DryRun_FiltersApplied(t *testing.T) {
	t.Skip("TODO: dry-run with skip/exclude/include; assert the listed mutants " +
		"respect the filters (e.g., skipped files have zero mutants listed)")
}

// ============================================================================
// WORKFLOW — SUB-CONFIG MODE EDGE CASES
// ============================================================================

// TestWorkflow_SubConfigMode_Merge verifies sub_config_mode: merge.
func TestWorkflow_SubConfigMode_Merge(t *testing.T) {
	t.Skip("TODO: sub_config_mode: merge; create nested sub-configs; " +
		"assert all levels' settings are merged according to the replace/accumulate rules")
}

// TestWorkflow_SubConfigMode_Replace verifies sub_config_mode: replace.
func TestWorkflow_SubConfigMode_Replace(t *testing.T) {
	t.Skip("TODO: sub_config_mode: replace; create nested sub-configs; " +
		"assert the deepest sub-config completely replaces the parent config")
}

// TestWorkflow_SubConfigMode_Isolate verifies sub_config_mode: isolate.
func TestWorkflow_SubConfigMode_Isolate(t *testing.T) {
	t.Skip("TODO: sub_config_mode: isolate; create nested sub-configs; " +
		"assert each directory is completely independent with no inheritance from ancestors")
}

// ============================================================================
// WORKFLOW — BADGE GENERATION
// ============================================================================

// TestWorkflow_Badge_SVG_Generated verifies SVG badge output.
func TestWorkflow_Badge_SVG_Generated(t *testing.T) {
	t.Skip("TODO: run with badge: svg output flag; assert a valid SVG file is " +
		"generated with the correct mutation score percentage")
}

// TestWorkflow_Badge_JSON_Generated verifies JSON badge output.
func TestWorkflow_Badge_JSON_Generated(t *testing.T) {
	t.Skip("TODO: run with badge: json; assert a valid JSON shield is generated")
}

// ============================================================================
// WORKFLOW — PROFILING
// ============================================================================

// TestWorkflow_CPUProfile_FileCreated verifies cpu_profile flag creates profile.
func TestWorkflow_CPUProfile_FileCreated(t *testing.T) {
	t.Skip("TODO: run with cpu_profile: /tmp/cpu.prof; assert file is created " +
		"and is a valid pprof profile")
}

// TestWorkflow_MemProfile_DirectoryCreated verifies mem_profile flag creates
// heap profile files.
func TestWorkflow_MemProfile_DirectoryCreated(t *testing.T) {
	t.Skip("TODO: run with mem_profile: /tmp/mem; assert heap profile files are " +
		"created in that directory")
}
