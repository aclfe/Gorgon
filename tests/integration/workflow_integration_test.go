//go:build integration
// +build integration

package integration

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// WORKFLOW BASIC PIPELINE
// ============================================================================

// TestWorkflow_OverallScoreValidation test verified. Works. 
func TestWorkflow_OverallScoreValidation(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	outputDir := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_OverallScoreValidation/output")
	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	stats := runPipelineWithOutputs(t, repoRoot, outputDir)
	jsonPath := filepath.Join(outputDir, "report.json")
	htmlDir := filepath.Join(outputDir, "report.html")
	junitPath := filepath.Join(outputDir, "report.xml")
	sarifPath := filepath.Join(outputDir, "report.sarif")
	textPath := filepath.Join(outputDir, "report.txt")

	jsonStats, err := extractStatsFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("failed to extract stats from JSON: %v", err)
	}

	htmlStats, err := extractStatsFromHTML(htmlDir)
	if err != nil {
		t.Fatalf("failed to extract stats from HTML: %v", err)
	}

	junitStats, err := extractStatsFromJUnit(junitPath)
	if err != nil {
		t.Fatalf("failed to extract stats from JUnit: %v", err)
	}

	sarifStats, err := extractStatsFromSARIF(sarifPath)
	if err != nil {
		t.Fatalf("failed to extract stats from SARIF: %v", err)
	}

	textStats, err := extractStatsFromText(textPath)
	if err != nil {
		t.Fatalf("failed to extract stats from text: %v", err)
	}

	reference := jsonStats

	var allDiscrepancies []string

	if discrepancies := compareStats(reference, htmlStats, "HTML"); len(discrepancies) > 0 {
		allDiscrepancies = append(allDiscrepancies, discrepancies...)
	}
	if discrepancies := compareStats(reference, junitStats, "JUnit"); len(discrepancies) > 0 {
		allDiscrepancies = append(allDiscrepancies, discrepancies...)
	}
	if discrepancies := compareStats(reference, sarifStats, "SARIF"); len(discrepancies) > 0 {
		allDiscrepancies = append(allDiscrepancies, discrepancies...)
	}
	if discrepancies := compareStats(reference, textStats, "Text"); len(discrepancies) > 0 {
		allDiscrepancies = append(allDiscrepancies, discrepancies...)
	}

	if discrepancies := compareStats(reference, stats, "Internal"); len(discrepancies) > 0 {
		allDiscrepancies = append(allDiscrepancies, discrepancies...)
	}

	if len(allDiscrepancies) > 0 {
		t.Errorf("Mutation score/stats inconsistent across output formats:\n  %s",
			strings.Join(allDiscrepancies, "\n  "))
	}

	expectedScore := calculateExpectedScore(reference)
	if math.Abs(reference.Score-expectedScore) > 0.01 {
		t.Errorf("Score formula incorrect in outputs: reported=%.2f, expected=%.2f (Killed=%d, Survived=%d, Untested=%d, Timeout=%d)",
			reference.Score, expectedScore,
			reference.Killed, reference.Survived, reference.Untested, reference.Timeout)
	}

	// Verify internal score calculation matches formula
	// Formula: Score = Killed / (Killed + Survived + Untested + Timeout) * 100
	// CompileErrors, RuntimeErrors, and Invalid are excluded from denominator
	internalExpected := calculateExpectedScore(stats)
	if math.Abs(stats.Score-internalExpected) > 0.01 {
		t.Errorf("Score formula incorrect internally: reported=%.2f, expected=%.2f (Killed=%d, Survived=%d, Untested=%d, Timeout=%d)",
			stats.Score, internalExpected,
			stats.Killed, stats.Survived, stats.Untested, stats.Timeout)
	}

	// Verify CompileErrors are excluded from score calculation
	// Using rounded comparison to handle floating point precision properly
	if stats.CompileErrors > 0 {
		incorrectDenom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout + stats.CompileErrors
		incorrectScore := float64(stats.Killed) / float64(incorrectDenom) * 100
		actualRounded := math.Round(stats.Score*100000) / 100000
		incorrectRounded := math.Round(incorrectScore*100000) / 100000
		if actualRounded == incorrectRounded {
			t.Errorf("CompileErrors appear to be included in score denominator: score=%.5f, incorrect_formula_score=%.5f",
				stats.Score, incorrectScore)
		}
	}

	if stats.RuntimeErrors > 0 {
		incorrectDenom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout + stats.RuntimeErrors
		incorrectScore := float64(stats.Killed) / float64(incorrectDenom) * 100
		actualRounded := math.Round(stats.Score*100000) / 100000
		incorrectRounded := math.Round(incorrectScore*100000) / 100000
		if actualRounded == incorrectRounded {
			t.Errorf("RuntimeErrors appear to be included in score denominator: score=%.5f, incorrect_formula_score=%.5f",
				stats.Score, incorrectScore)
		}
	}

	if stats.Invalid > 0 {
		incorrectDenom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout + stats.Invalid
		incorrectScore := float64(stats.Killed) / float64(incorrectDenom) * 100
		actualRounded := math.Round(stats.Score*100000) / 100000
		incorrectRounded := math.Round(incorrectScore*100000) / 100000
		if actualRounded == incorrectRounded {
			t.Errorf("Invalid mutants appear to be included in score denominator: score=%.5f, incorrect_formula_score=%.5f",
				stats.Score, incorrectScore)
		}
	}

	sum := stats.Killed + stats.Survived + stats.CompileErrors + stats.RuntimeErrors +
		stats.Timeout + stats.Untested + stats.Invalid
	if sum != stats.Total {
		t.Errorf("Category sum %d != Total %d (Killed=%d, Survived=%d, CompileErrors=%d, RuntimeErrors=%d, Timeout=%d, Untested=%d, Invalid=%d)",
			sum, stats.Total,
			stats.Killed, stats.Survived, stats.CompileErrors, stats.RuntimeErrors,
			stats.Timeout, stats.Untested, stats.Invalid)
	}

	denom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout
	if denom > 0 {
		expected := float64(stats.Killed) / float64(denom) * 100
		if d := stats.Score - expected; d < -0.01 || d > 0.01 {
			t.Errorf("score %.4f != formula result %.4f (killed=%d denom=%d)",
				stats.Score, expected, stats.Killed, denom)
		}
	}

	if stats.TotalErrors != stats.CompileErrors+stats.RuntimeErrors {
		t.Errorf("TotalErrors mismatch: TotalErrors=%d, CompileErrors=%d, RuntimeErrors=%d (expected TotalErrors=%d)",
			stats.TotalErrors, stats.CompileErrors, stats.RuntimeErrors,
			stats.CompileErrors+stats.RuntimeErrors)
	}

	if stats.Total == 0 {
		t.Fatal("no mutants produced — repo traversal may be broken")
	}
}

// TestWorkflow_NoMutantsLost.
// Bug 1: Duplicate ID - Verified
// Bug 2: Clear status for first mutant - Works verified. 
// Bug 3: Empty file property - Go routine failed. 

// --- FAIL: TestWorkflow_NoMutantsLost (76.60s)
// panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
// [signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x821051]

// goroutine 8 [running]:
// testing.tRunner.func1.2({0x8c13e0, 0xd192c0})
// 	/usr/local/go/src/testing/testing.go:1974 +0x232
// testing.tRunner.func1()
// 	/usr/local/go/src/testing/testing.go:1977 +0x349
// panic({0x8c13e0?, 0xd192c0?})
// 	/usr/local/go/src/runtime/panic.go:860 +0x13a
// go/token.(*File).Name(...)
// 	/usr/local/go/src/go/token/position.go:117

// Bug 4: Skip mutant ID 3 in HTML report - Verified. 
// Bug 5: Duplicate mutant ID 4 in JSON - Verified. 
func TestWorkflow_NoMutantsLost(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	outputDir := filepath.Join(repoRoot, "tests/integration/testdata/TestWorkflow_NoMutantsLost/output")

	os.RemoveAll(outputDir)
	os.MkdirAll(outputDir, 0755)

	// Clean up output files after test completes to prevent repo bloat
	defer os.RemoveAll(outputDir)

	mutants, stats := runPipelineWithMutantTracking(t, repoRoot, outputDir)

	if len(mutants) == 0 {
		t.Fatal("no mutants produced — repo traversal may be broken")
	}

	// 1. Verify ID completeness: all IDs 1..N appear exactly once
	idErrors := checkIDCompleteness(mutants)
	if len(idErrors) > 0 {
		t.Errorf("ID completeness check failed:\n  %s", strings.Join(idErrors, "\n  "))
	}

	// 2. Verify each mutant has valid properties
	var validationErrors []string
	for _, m := range mutants {
		errors := validateMutant(m)
		validationErrors = append(validationErrors, errors...)
	}
	if len(validationErrors) > 0 {
		t.Errorf("Mutant validation failed:\n  %s", strings.Join(validationErrors, "\n  "))
	}

	// 3. Verify all mutants are categorized (match Total count)
	if len(mutants) != stats.Total {
		t.Errorf("Mutant count mismatch: tracked=%d, stats.Total=%d", len(mutants), stats.Total)
	}

	// 4. Cross-validate all output formats contain the same mutants
	jsonPath := filepath.Join(outputDir, "report.json")
	htmlDir := filepath.Join(outputDir, "report.html")
	textPath := filepath.Join(outputDir, "report.txt")

	jsonMutants, err := extractMutantsFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("Failed to extract mutants from JSON: %v", err)
	}

	htmlMutants, err := extractMutantsFromHTML(htmlDir)
	if err != nil {
		t.Fatalf("Failed to extract mutants from HTML: %v", err)
	}

	textMutants, err := extractMutantsFromText(textPath)
	if err != nil {
		t.Fatalf("Failed to extract mutants from Text: %v", err)
	}

	var crossFormatErrors []string

	t.Logf("DEBUG: Internal mutants: %d, JSON: %d, HTML: %d, Text: %d", len(mutants), len(jsonMutants), len(htmlMutants), len(textMutants))

	if len(jsonMutants) > 0 {
		if discrepancies := compareMutantLists(mutants, jsonMutants, "JSON"); len(discrepancies) > 0 {
			crossFormatErrors = append(crossFormatErrors, discrepancies...)
		}
	}

	if len(htmlMutants) > 0 {
		if discrepancies := compareMutantLists(mutants, htmlMutants, "HTML"); len(discrepancies) > 0 {
			crossFormatErrors = append(crossFormatErrors, discrepancies...)
		}
	}

	if len(textMutants) > 0 {
		if discrepancies := compareMutantLists(mutants, textMutants, "Text"); len(discrepancies) > 0 {
			crossFormatErrors = append(crossFormatErrors, discrepancies...)
		}
	}

	if len(crossFormatErrors) > 0 {
		t.Errorf("Cross-format validation failed:\n  %s", strings.Join(crossFormatErrors, "\n  "))
	}
}

// ============================================================================
// WORKFLOW TEST SUITES
// ============================================================================

// TestWorkflow_ExternalSuitesActuallyKillMutations verifies external suites kill mutations
func TestWorkflow_ExternalSuitesActuallyKillMutations(t *testing.T) {
	t.Skip("TODO: Verify external test suites run and kill mutations")
}

// TestWorkflow_ExternalSuiteRunModes verifies external suite run modes (after_unit, only, etc.)
func TestWorkflow_ExternalSuiteRunModes(t *testing.T) {
	t.Skip("TODO: Verify external suite run modes work correctly")
}

// TestWorkflow_ExternalSuiteTags verifies external suite build tags
func TestWorkflow_ExternalSuiteTags(t *testing.T) {
	t.Skip("TODO: Verify external suite build tags are applied")
}

// TestBothTestSuites_BothEnabled verifies that both unit and external tests run when both are enabled
func TestBothTestSuites_BothEnabled(t *testing.T) {
	t.Skip("TODO: Verify if both unit and external tests run when both are enabled. This is currently difficult to assert without stronger output from gorgon.")
}

// TestBothTestSuites_ExternalOnly verifies that only external tests run when unit tests are disabled
func TestBothTestSuites_ExternalOnly(t *testing.T) {
	t.Skip("TODO: Verify if only external tests run when unit tests are disabled. This is currently difficult to assert without stronger output from gorgon.")
}

// TestBothTestSuites_UnitOnly verifies that only unit tests run when external tests are disabled
func TestBothTestSuites_UnitOnly(t *testing.T) {
	t.Skip("TODO: Verify if only unit tests run when external tests are disabled. This is currently difficult to assert without stronger output from gorgon.")
}

// TestBothTestSuites_NoneEnabled verifies that no tests run when both are disabled
func TestBothTestSuites_NoneEnabled(t *testing.T) {
	t.Skip("TODO: Verify if no tests run when both are disabled. This is currently difficult to assert without stronger output from gorgon.")
}

func TestBothTestSuites_ValidKilling(t * tseting.T) {
	t.Skip("TODO: Verify that they're actually killing what they should be. Checks for FP, FN, TP, TN")
}

// ============================================================================
// WORKFLOW OPERATORS
// ============================================================================

// TestWorkflow_DifferentOperatorsProduceDifferentResults verifies different operators produce different results
func TestWorkflow_DifferentOperatorsProduceDifferentResults(t *testing.T) {
	t.Skip("TODO: Verify different operators produce different mutation results")
}

func TestWorflow_AllOperatorsCanProduceMutations(t *testing.T) {
	t.Skip("TODO: Verify that they're all enabled and working to make mutations")
	// frankly a future might be to ensure that there's no error mutations being generated.
	// this is a highly ambitious goal consider handlers.go would nee chanes in order to ensure mutations are done properly. 
}

// ============================================================================
// WORKFLOW SCHEMATA TRANSFORMATION
// ============================================================================

// TestWorkflow_SchemataCompilationSuccess verifies schemata transformation produces compilable code everywhere in the repo
func TestWorkflow_SchemataCompilationSuccess(t *testing.T) {
	t.Skip("TODO: Verify schemata-transformed code compiles")
}

// TestWorkflow_PreflightCatchesBaselineErrors verifies preflight catches baseline errors
func TestWorkflow_PreflightCatchesBaselineErrors(t *testing.T) {
	t.Skip("TODO: Verify preflight catches pre-existing type errors")
}

// TestWorkflow_MultiValueReturnNoCompilationError verifies multi-value returns don't cause compilation errors
func TestWorkflow_MultiValueReturnNoCompilationError(t *testing.T) {
	t.Skip("TODO: Verify multi-value return statements work with schemata")
}

func TestWorkflow_AllPreflightPhasesWork(t *testing.T) {
	t.Skip("TODO: We'll verify that all phases are able to filter mutations properly and not just be dummy.")
}

// ============================================================================
// WORKFLOW CACHING
// ============================================================================

// TestWorkflow_CacheActuallyWorks verifies cache skips re-running identical mutants
func TestWorkflow_CacheActuallyWorks(t *testing.T) {
	t.Skip("TODO: Verify cache correctly skips re-running identical mutants")
}

// TestWorkflow_CacheWithDiff verifies cache + diff interaction
func TestWorkflow_CacheWithDiff(t *testing.T) {
	t.Skip("TODO: Verify cache works correctly with diff filtering")
}

// TestWorkflow_CacheWithDiff verifies cache + diff interaction + baseline
func TestWorkflow_CacheWithDiffAndBaseline(t *testing.T) {
	t.Skip("TODO: Verify cache works correctly with diff filtering and baseline")
}

// TestWorkflow_CacheWithDiff verifies cache + diff interaction + baseline + directory rules / subconfigs
func TestWorkflow_CacheWithDiffAndBaseline_Subconfig(t *testing.T) {
	t.Skip("TODO: Verify cache works correctly with diff filtering and baseline + subconfig specifications")
}

// TestWorkflow_CacheWithDiff verifies cache + diff interaction + baseline + directory rules / subconfigs + org policy
func TestWorkflow_CacheWithDiffAndBaseline_Subconfig_Orgpolicy(t *testing.T) {
	t.Skip("TODO: Verify cache works correctly with diff filtering and baseline + subconfig specifications + org policy")
}

// TestWorkflow_CacheWithDiff verifies cache + diff interaction + baseline + org policy
func TestWorkflow_CacheWithDiffAndBaseline_orgpolicy(t *testing.T) {
	t.Skip("TODO: Verify cache works correctly with diff filtering and baseline + org policy")
}

// ============================================================================
// WORKFLOW DIFF FILTERING
// ============================================================================

// TestWorkflow_DiffFilteringWorks verifies -diff flag filters mutations
func TestWorkflow_DiffFilteringWorks(t *testing.T) {
	t.Skip("TODO: Verify -diff only mutates changed lines")
}

// TestWorkflow_DiffPathFile verifies -diff=path/to/patch works
func TestWorkflow_DiffPathFile(t *testing.T) {
	t.Skip("TODO: Verify -diff accepts patch file path")
}

// TestWorkflow_DiffPathFile verifies many types of inputs / arguments work. 
func TestWorkflow_DiffPathFileInputs(t *testing.T) {
	t.Skip("TODO: Verify many inputs / combinations work.")
}

// ============================================================================
// WORKFLOW CONCURRENCY
// ============================================================================

// TestWorkflow_ConcurrentExecutionSafe verifies concurrent execution produces consistent results
// I know there is an issue right now, it's quite flaky frankly speaking. 
func TestWorkflow_ConcurrentExecutionSafe(t *testing.T) {
	t.Skip("TODO: Verify concurrent execution is deterministic")
}

// TestWorkflow_ConcurrentLimit verifies -concurrent flag limits parallelism
func TestWorkflow_ConcurrentLimit(t *testing.T) {
	t.Skip("TODO: Verify -concurrent limits parallelism correctly")
}

// ============================================================================
// WORKFLOW MUTANT HANDLING
// ============================================================================

// TestWorkflow_SurvivedMutantsClassified verifies survived mutants are classified correctly
func TestWorkflow_SurvivedMutantsClassified(t *testing.T) {
	t.Skip("TODO: Verify survived mutants are marked as 'survived' status")
}

// TestWorkflow_KilledMutantsClassified verifies killed mutants are classified correctly
func TestWorkflow_KilledMutantsClassified(t *testing.T) {
	t.Skip("TODO: Verify killed mutants are marked as 'killed' status")
}

// TestWorkflow_CompileErrorMutantsClassified verifies compile error mutants are classified correctly
func TestWorkflow_CompileErrorMutantsClassified(t *testing.T) {
	t.Skip("TODO: Verify compile error mutants are marked as 'compile error' status")
}

// TestWorkflow_RuntimeErrorMutantsClassified verifies runtime error mutants are classified correctly
func TestWorkflow_RuntimeErrorMutantsClassified(t *testing.T) {
	t.Skip("TODO: Verify runtime error mutants are marked as 'runtime error' status")
}

// TestWorkflow_TimeoutMutantsClassified verifies timeout mutants are classified correctly
func TestWorkflow_TimeoutMutantsClassified(t *testing.T) {
	t.Skip("TODO: Verify timed-out mutants are marked as 'timeout' status")
}

// TestWorkflow_UntestedMutatantsClassified verifies untested mutants are classified correctly
func TestWorkflow_UntestedMutatantsClassified(t *testing.T) {
	t.Skip("TODO: Verify untested mutants are marked as 'untested' status")
}

// TestWorkflow_InvalidMutantsClassified verifies invalid mutants are classified correctly
func TestWorkflow_InvalidMutantsClassified(t *testing.T) {
	t.Skip("TODO: Verify invalid mutants are marked as 'invalid' status")
}

// frankly I think I can merged all of this into 1 test, but we'll keep multiple just for a reminder. 
// THe main issue is that they're not being attributed correctly I believe.

// ============================================================================
// WORKFLOW FILTERING
// ============================================================================

// TestWorkflow_SkipRulesRespected verifies -skip and -exclude flags skip files
func TestWorkflow_SkipRulesRespected(t *testing.T) {
	t.Skip("TODO: Verify -skip and -exclude flags work correctly")
}

// TestWorkflow_SkipFunc verifies -skip-func flag skips specific functions
func TestWorkflow_SkipFunc(t *testing.T) {
	t.Skip("TODO: Verify -skip-func skips specific functions")
}

// TestWorkflow_IncludeRules verifies -include flag filters files
func TestWorkflow_IncludeRules(t *testing.T) {
	t.Skip("TODO: Verify -include filters files correctly")
}

// TestWorkflow_TestsFlagFilters verifies -tests flag filters test files
func TestWorkflow_TestsFlagFilters(t *testing.T) {
	t.Skip("TODO: Verify -tests filters test files correctly")
}

// TestWorkflow_TestsFlagFilters verifies all skipping filters work together
func TestWorkflow_TestsSkipFiltersAll(t *testing.T) {
	t.Skip("TODO: Verify -all skipping filters work together")
}

// ============================================================================
// WORKFLOW KILL ATTRIBUTION
// ============================================================================

// TestWorkflow_KillAttributionCorrect verifies KilledBy field identifies correct test
func TestWorkflow_KillAttributionCorrect(t *testing.T) {
	t.Skip("TODO: Verify KilledBy field identifies correct test")
}

// ============================================================================
// WORKFLOW WORKSPACE MULTI-MODULE
// ============================================================================

// TestWorkflow_WorkspaceMultiModulePreserved verifies go.work multi-module layout is preserved
func TestWorkflow_WorkspaceMultiModulePreserved(t *testing.T) {
	t.Skip("TODO: Verify go.work workspace layout is preserved")
}

// sensitive feature, needs way more placeholder tests in diverse settings and complex configurations. Subjected to increase.

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
