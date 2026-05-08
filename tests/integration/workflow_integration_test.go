//go:build integration
// +build integration

package integration

import (
	"path/filepath"
	"strings"
	"testing"

	coretesting "github.com/aclfe/gorgon/internal/core"
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

// TestWorkflow_WorkspaceMultiModulePreserved verifies go.work multi-module layout is preserved
func TestWorkflow_WorkspaceMultiModulePreserved(t *testing.T) {
	t.Skip("TODO: Verify go.work workspace layout is preserved")
}

// ============================================================================
// WORKFLOW DIR RULES
// ============================================================================

// TestWorkflow_DirRulesWhitelistBlacklist verifies dir_rules whitelist/blacklist
func TestWorkflow_DirRulesWhitelistBlacklist(t *testing.T) {
	t.Skip("TODO: Verify dir_rules whitelist/blacklist work correctly")
}

// sensitive feature, needs way more placeholder tests in diverse settings and complex configurations (especially sub config and org policy etc and many others)
// Subjected to increase.

// ============================================================================
// WORKFLOW SUB-CONFIG INHERITANCE
// ============================================================================

// TestWorkflow_SubConfigInheritance verifies sub-configs inherit parent settings
func TestWorkflow_SubConfigInheritance(t *testing.T) {
	t.Skip("TODO: Verify sub-configs inherit and override parent settings")
}

// sensitive feature, needs way more placeholder tests in diverse settings and complex configurations (especially sub config and org policy etc and many others)
// Subjected to increase.


// ============================================================================
// WORKFLOW BASELINE
// ============================================================================

// TestWorkflow_BaselineNoRegression verifies -no-regression mode with baseline
func TestWorkflow_BaselineNoRegression(t *testing.T) {
	t.Skip("TODO: Verify baseline regression checking works")
}

// TestWorkflow_BaselineTolerance verifies -baseline-tolerance allows drift
func TestWorkflow_BaselineTolerance(t *testing.T) {
	t.Skip("TODO: Verify baseline tolerance allows score drift")
}

// sensitive feature, needs way more placeholder tests in diverse settings and complex configurations (especially sub config and org policy etc and many others)
// Subjected to increase.


// ============================================================================
// WORKFLOW DRY RUN
// ============================================================================

// TestWorkflow_DryRunMode verifies -dry-run shows mutants without running tests
func TestWorkflow_DryRunMode(t *testing.T) {
	t.Skip("TODO: Verify -dry-run shows mutants without running tests")
}

// ============================================================================
// WORKFLOW PROGRESS BAR
// ============================================================================

// TestWorkflow_ProgressBarLifecycle verifies -progbar shows progress
func TestWorkflow_ProgressBarLifecycle(t *testing.T) {
	t.Skip("TODO: Verify progress bar displays and updates correctly")
}

// ============================================================================
// WORKFLOW ORG POLICY
// ============================================================================

// TestWorkflow_OrgPolicyEnforcement verifies gorgon-org.yml policy enforcement
func TestWorkflow_OrgPolicyEnforcement(t *testing.T) {
	t.Skip("TODO: Verify org policy constraints are enforced")
}

// ============================================================================
// WORKFLOW THRESHOLD CHECKING
// ============================================================================

// TestWorkflow_ThresholdChecking verifies -threshold flag fails correctly
func TestWorkflow_ThresholdChecking(t *testing.T) {
	t.Skip("TODO: Verify threshold checking fails when score is below threshold")
}

// TestWorkflow_ThresholdCheckingAllFiles verifies -threshold flags works on all output files
func TestWorkflow_ThresholdCheckingAllFiles(t *testing.T) {
	t.Skip("TODO: Verify threshold verifies -threshold flags works on all output files")
}


// ============================================================================
// WORKFLOW SUPPRESSIONS
// ============================================================================

// TestWorkflow_SuppressionsWork verifies inline suppressions work
func TestWorkflow_SuppressionsWork(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore and config suppressions work")
}

// TestWorkflow_SuppressionsWork verifies inline suppressions work
func TestWorkflow_SuppressionsWorkUnderSubconfig(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore and config suppressions work under subconfig")
}

// TestWorkflow_SuppressionsWork verifies inline suppressions work
func TestWorkflow_SuppressionsWorkUnderOrgpolicy(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore and config suppressions work under org policy")
}

// TestWorkflow_SuppressionsWork verifies inline suppressions work
func TestWorkflow_SuppressionsWorkUnderSubconfigAndOrgpolicy(t *testing.T) {
	t.Skip("TODO: Verify //gorgon:ignore and config suppressions work under sub config and org policy")
}

// ============================================================================
// WORKFLOW CONFIGURATION
// ============================================================================

// TestWorkflow_ConfigureGoVersion verifies go_version config option
func TestWorkflow_ConfigureGoVersion(t *testing.T) {
	t.Skip("TODO: Verify go_version config overrides detected version")
}

// ============================================================================
// WORKFLOW LARGE FILE HANDLING
// ============================================================================

// TestWorkflow_ChunkLargeFiles verifies chunk_large_files prevents OOM
func TestWorkflow_ChunkLargeFiles(t *testing.T) {
	t.Skip("TODO: Verify chunk_large_files prevents OOM on large files") // I might or might not keep this. I'm not sure if we really need chunking.
	// The main issue before was the memory management in handlers was blowing up with recursive calls and badly written code causing an exponential blast in memory.
	// The memory issue isn't persisting, but that raises a concern, do we need chunking if the memry usage even ona 500 line file isn't anything significant.
}

// ============================================================================
// WORKFLOW MEMORY CHECKING
// ============================================================================

// TestWorkflow_MemoryUsage verifies memory usage isn't OOM-ing or close to OOM-ing. If it is, that's 100% an issue with code. Inherently, gorgon uses less memory.
func TestWorkflow_MemoryUsage(t *testing.T) {
	t.Skip("TODO: Verify memory usage isn't OOM-ing or close to OOM-ing")
}
