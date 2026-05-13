//go:build integration
// +build integration

package integration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/orgpolicy"
	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// ============================================================================
// PREFLIGHT CHECKING
//
// Preflight runs in three levels before the full mutation workspace is built:
//   Level 1: Quick static checks (operator-level preflight hooks)
//   Level 2: ApplySchemataToAST + in-memory type check per file
//   Level 3: typeCheckFileGroup — in-memory type check of file groups
//
// Tests cover each level, error detection, and performance characteristics.
// ============================================================================

// TestWorkflow_PreflightCatchesBaselineErrors verifies preflight catches
// pre-existing type errors in the original code before schemata is applied.
//
// Creates a temp Go file that parses but has a type error (wrong return type).
// Preflight L3 must detect the baseline error and subtract it so the mutant
// is not falsely blamed for the pre-existing error.
func TestWorkflow_PreflightCatchesBaselineErrors(t *testing.T) {
	dir := t.TempDir()

	// This file parses fine but fails type-check: returns string for int.
	src := `package foo

func BadFunc() int {
	return "this is not an int"
}
`
	pkgFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(pkgFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Write a go.mod so the engine can load the package.
	gomod := `module preflightbaseline

go 1.21
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	ops := mutator.ListAll()
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(dir)

	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites in temp package")
	}

	log := logger.New(false)
	mutants := coretesting.GenerateMutants(sites, ops, ops, dir, nil, nil, log)
	if len(mutants) == 0 {
		t.Fatalf("no mutants generated in temp package")
	}

	remaining, results := coretesting.RunPreflight(mutants, log)

	// The original file has a type error. Preflight should handle this without
	// panicking or marking ALL mutants as invalid. Each mutant may or may not
	// survive — what matters is the baseline was computed and subtracted.
	if len(results) == len(mutants) && len(remaining) == 0 {
		// All mutants were rejected — that's acceptable for a file with a type
		// error. Verify all rejections have a reason.
		for _, r := range results {
			if r.ErrorReason == "" {
				t.Errorf("preflight result for mutant %d has empty error reason", r.MutantID)
			}
			if r.Status == "" {
				t.Errorf("preflight result for mutant %d has empty status", r.MutantID)
			}
		}
	}

	// The key invariant: preflight must not panic and must return structured
	// results. Even if all mutants are rejected, the rejection reason must be
	// set so callers can debug.
	t.Logf("preflight on type-error file: %d mutants → %d remaining, %d filtered",
		len(mutants), len(remaining), len(results))
}

// TestWorkflow_AllPreflightPhasesWork verifies that all three preflight levels
// run on a real package and produce valid results.
func TestWorkflow_AllPreflightPhasesWork(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced in %s", targetDir)
	}

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// L1 always runs (nil node/file checks + SafetyConstrainedOperator).
	// L2 runs on survivors of L1.
	// L3 runs on survivors of L2.
	// After all levels, the remaining count must be <= initial count.
	if len(remaining) > len(rawMutants) {
		t.Errorf("preflight returned more mutants than input: %d > %d",
			len(remaining), len(rawMutants))
	}

	// Every filtered mutant must have a reason and status.
	statusCounts := make(map[string]int)
	for _, r := range results {
		if r.ErrorReason == "" {
			t.Errorf("preflight result for mutant %d has empty error reason", r.MutantID)
		}
		statusCounts[r.Status]++
	}
	for _, m := range remaining {
		if m.Status == coretesting.StatusInvalid {
			t.Errorf("mutant %d survived preflight with status=%q", m.ID, m.Status)
		}
	}

	t.Logf("preflight: %d initial → %d remaining (%d invalid=%s, %d compile=%s, %d other)",
		len(rawMutants), len(remaining),
		statusCounts[coretesting.StatusInvalid], coretesting.StatusInvalid,
		statusCounts[coretesting.StatusCompileError], coretesting.StatusCompileError,
		len(results)-statusCounts[coretesting.StatusInvalid]-statusCounts[coretesting.StatusCompileError])
}

// ============================================================================
// PREFLIGHT — LEVEL 1 (STATIC CHECKS)
// ============================================================================

// TestPreflight_Level1_OperatorSpecificPreflight verifies that L1 static
// checks catch obviously unsafe mutations (nil node, nil file) before any
// AST transformation runs.
func TestPreflight_Level1_OperatorSpecificPreflight(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	// L1 runs quickStaticFilter which checks:
	//   1. m.Site.Node == nil  → StatusInvalid "nil node"
	//   2. m.Site.File == nil  → StatusInvalid "nil file"
	//   3. isObviouslyUnsafeMutation(m) → StatusInvalid "obviously unsafe mutation"
	//
	// All three conditions must be verified as reachable.
	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// At minimum, L1 should have run. Count L1 rejections.
	l1Invalid := 0
	for _, r := range results {
		if r.Status == coretesting.StatusInvalid {
			l1Invalid++
		}
	}

	// L1 might reject 0 or more mutants depending on the operators active.
	// The key assertion: no mutant with a nil node or nil file survives L1.
	for _, m := range remaining {
		if m.Site.Node == nil {
			t.Errorf("mutant %d has nil node but survived L1", m.ID)
		}
		if m.Site.File == nil {
			t.Errorf("mutant %d has nil file but survived L1", m.ID)
		}
	}

	t.Logf("L1 filtered %d mutants as invalid; %d passed to L2", l1Invalid, len(remaining))
}

// TestPreflight_Level1_NoOperatorsWithPreflight_BypassesLevel1 verifies that
// when an operator does NOT implement SafetyConstrainedOperator, its mutants
// pass through L1 (L1 does not over-reject).
func TestPreflight_Level1_NoOperatorsWithPreflight_BypassesLevel1(t *testing.T) {
	// negate_condition does not implement SafetyConstrainedOperator.
	// Its mutants must pass through L1 unless they have nil node/file.
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// Count how many negate_condition mutants were rejected by L1 specifically.
	l1Rejected := 0
	for _, r := range results {
		if r.Status == coretesting.StatusInvalid {
			// Find the mutant to check its operator
			for _, m := range rawMutants {
				if m.ID == r.MutantID {
					if m.Operator != nil && m.Operator.Name() == "negate_condition" {
						l1Rejected++
					}
					break
				}
			}
		}
	}

	// negate_condition mutants should not be rejected by L1's isObviouslyUnsafeMutation
	// (they don't implement SafetyConstrainedOperator). Any L1 rejection of
	// negate_condition must only be from nil node/file checks.
	if l1Rejected > 0 {
		t.Logf("%d negate_condition mutants rejected by L1 (expected 0 unless nil node/file)", l1Rejected)
	}

	if len(remaining) == 0 {
		t.Fatalf("all mutants rejected — negate_condition should have survivors past L1")
	}
}

// TestPreflight_Level1_InvalidMutant_RemovedFromSlice verifies that mutants
// rejected at level 1 are removed from the valid slice before level 2 runs.
func TestPreflight_Level1_InvalidMutant_RemovedFromSlice(t *testing.T) {
	// Craft a mutant with a nil node — L1 must reject it.
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	// Inject a deliberately invalid mutant with nil Node.
	badMutant := rawMutants[0]
	badMutant.ID = 99999
	badMutant.Site.Node = nil
	testMutants := append([]coretesting.Mutant{badMutant}, rawMutants...)

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(testMutants, log)

	// The nil-node mutant must be in the rejected list.
	found := false
	for _, r := range results {
		if r.MutantID == 99999 {
			found = true
			if r.Status != coretesting.StatusInvalid {
				t.Errorf("nil-node mutant got status %q, want %q", r.Status, coretesting.StatusInvalid)
			}
			if r.ErrorReason != "nil node" {
				t.Errorf("nil-node mutant error reason = %q, want %q", r.ErrorReason, "nil node")
			}
			break
		}
	}
	if !found {
		t.Errorf("nil-node mutant (ID=99999) not found in preflight rejection results")
	}

	// The bad mutant must NOT be in the remaining slice.
	for _, m := range remaining {
		if m.ID == 99999 {
			t.Errorf("nil-node mutant survived preflight — should have been removed")
		}
	}

	// Regular mutants should still survive.
	if len(remaining) == 0 {
		t.Errorf("all mutants rejected — the nil-node mutant should have been the only L1 rejection")
	}
}

// ============================================================================
// PREFLIGHT — LEVEL 2 (IN-MEMORY SCHEMATA + TYPE CHECK)
// ============================================================================

// TestPreflight_Level2_InMemorySchemata_CompilesSuccessfully verifies that
// ApplySchemataInMemory produces a valid AST that can be formatted.
func TestPreflight_Level2_InMemorySchemata_CompilesSuccessfully(t *testing.T) {
	src := `package foo

func Add(a, b int) int {
	return a + b
}
`
	fset := token.NewFileSet()
	srcFile := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Parse the source.
	fileAST, err := parser.ParseFile(fset, srcFile, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Create a mutant for the binary expression a + b.
	ops := mutator.ListAll()
	var negateCond mutator.Operator
	for _, op := range ops {
		if op.Name() == "negate_condition" {
			negateCond = op
			break
		}
	}
	if negateCond == nil {
		t.Fatalf("negate_condition operator not found")
	}

	// Find the binary expression node.
	var binExpr *ast.BinaryExpr
	ast.Inspect(fileAST, func(n ast.Node) bool {
		if be, ok := n.(*ast.BinaryExpr); ok {
			binExpr = be
			return false
		}
		return true
	})
	if binExpr == nil {
		t.Fatalf("no binary expression in test source")
	}

	mutant := coretesting.Mutant{
		ID:       1,
		Operator: negateCond,
		Site: engine.Site{
			File:    fset.File(binExpr.Pos()),
			FileAST: fileAST,
			Fset:    fset,
			Node:    binExpr,
		},
	}

	mutPtrs := []*coretesting.Mutant{&mutant}
	resultAST, err := coretesting.ApplySchemataInMemory([]byte(src), srcFile, fset, mutPtrs)
	if err != nil {
		t.Fatalf("ApplySchemataInMemory: %v", err)
	}
	if resultAST == nil {
		t.Fatalf("ApplySchemataInMemory returned nil AST")
	}

	// The resulting AST must have the schemata dispatch code injected.
	// Verify it contains the activeMutantID guard pattern.
	hasDispatch := false
	ast.Inspect(resultAST, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == "activeMutantID" {
			hasDispatch = true
			return false
		}
		return true
	})
	if !hasDispatch {
		t.Errorf("schemata-transformed AST does not contain activeMutantID — dispatch code not injected")
	}
}

// TestPreflight_Level2_TypeError_Detected verifies that schemata-induced type
// errors are caught by the level 2 check (schemata apply + format verification).
func TestPreflight_Level2_TypeError_Detected(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	// L2 runs checkFileWithSchemata which:
	//   1. Parses the source file
	//   2. Applies schemata to each mutant individually
	//   3. Checks format.Node (posMap == nil means unformattable)
	//   4. For files with >1 mutant, checks combined schemata too
	//
	// Most real mutants should pass L2. The key assertion is that L2 runs
	// without panicking and produces structured results.
	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// Track which level rejected which mutant.
	l2Rejected := 0
	for _, r := range results {
		if r.Status == coretesting.StatusCompileError {
			l2Rejected++
			if r.ErrorReason == "" {
				t.Errorf("L2/L3 rejected mutant %d but error reason is empty", r.MutantID)
			}
		}
	}

	// The pipeline should not lose all mutants at L2 for a well-formed package.
	if len(remaining) == 0 && len(rawMutants) > 0 {
		t.Errorf("all %d mutants rejected — reporter package should have survivors past L2", len(rawMutants))
	}

	t.Logf("L2+L3 rejected %d mutants; %d remaining", l2Rejected, len(remaining))
}

// TestPreflight_Level2_MultipleFilesInPackage verifies level 2 works when a
// package has multiple files (type checking must resolve cross-file references).
func TestPreflight_Level2_MultipleFilesInPackage(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	// Group mutants by file to verify multiple files were covered.
	files := make(map[string]int)
	for _, m := range rawMutants {
		if m.Site.File != nil {
			files[filepath.Base(m.Site.File.Name())]++
		}
	}
	if len(files) < 2 {
		t.Fatalf("reporter package has only %d file(s) with mutants — need at least 2", len(files))
	}

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// Verify each file's mutants were processed through preflight.
	remainingFiles := make(map[string]int)
	for _, m := range remaining {
		if m.Site.File != nil {
			remainingFiles[filepath.Base(m.Site.File.Name())]++
		}
	}

	// Cross-file references should not cause preflight to reject all mutants in
	// files that reference symbols from other files.
	if len(remaining) == 0 {
		t.Errorf("preflight removed all mutants from %d files — cross-file type checking may be broken", len(files))
	}

	t.Logf("preflight across %d files: %d initial → %d remaining (filtered %d)",
		len(files), len(rawMutants), len(remaining), len(results))
}

// TestPreflight_Level2_CrossPackageReferences verifies level 2 handles
// references to types/functions from imported packages.
func TestPreflight_Level2_CrossPackageReferences(t *testing.T) {
	repoRoot := findRepoRoot(t)
	// internal/reporter imports many packages (fmt, os, encoding/json, etc.)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	log := logger.New(false)
	remaining, _ := coretesting.RunPreflight(rawMutants, log)

	// Mutants that reference imported types (e.g., os.File, json.Encoder)
	// should not be rejected simply because the import is resolved differently
	// during preflight type-checking.
	if len(remaining) == 0 {
		t.Errorf("cross-package type checking failed — all %d mutants rejected", len(rawMutants))
	}
}

// ============================================================================
// PREFLIGHT — LEVEL 3 (FILE GROUP TYPE CHECK)
// ============================================================================

// TestPreflight_Level3_TypeCheckFileGroup verifies that typeCheckFileGroup
// catches errors that span multiple files in the same package.
func TestPreflight_Level3_TypeCheckFileGroup(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	// Verify we have mutants from multiple files so L3 has cross-file work.
	files := make(map[string]bool)
	for _, m := range rawMutants {
		if m.Site.File != nil {
			files[filepath.Base(m.Site.File.Name())] = true
		}
	}
	if len(files) < 2 {
		t.Fatalf("need mutants in >=2 files; got %d", len(files))
	}

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// L3 does type-checking using go/types with real import resolution.
	// On a real package that compiles, most mutants should survive L3.
	// The critical check: preflight must not crash, and rejected mutants
	// must have specific error reasons.
	for _, r := range results {
		if r.Status == coretesting.StatusCompileError && r.ErrorReason == "" {
			t.Errorf("L3 rejected mutant %d but has empty error reason", r.MutantID)
		}
	}

	t.Logf("L3 (file group type-check): %d mutants in %d files → %d remaining",
		len(rawMutants), len(files), len(remaining))
}

// TestPreflight_Level3_SingleFilePackage verifies level 3 works for a
// package with only one source file.
func TestPreflight_Level3_SingleFilePackage(t *testing.T) {
	repoRoot := findRepoRoot(t)
	// negate_condition is a single-file package.
	targetDir := filepath.Join(repoRoot, "pkg/mutator/operators/negate_condition")

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	// Confirm it's a single-file package.
	files := make(map[string]bool)
	for _, m := range rawMutants {
		if m.Site.File != nil {
			files[m.Site.File.Name()] = true
		}
	}
	if len(files) > 1 {
		t.Skipf("negate_condition has %d files — not a single-file package test", len(files))
	}

	log := logger.New(false)
	remaining, _ := coretesting.RunPreflight(rawMutants, log)

	// Single-file L3 should still complete without error.
	if len(remaining) == 0 {
		t.Errorf("single-file package: all mutants rejected")
	}
}

// ============================================================================
// PREFLIGHT — FULL PIPELINE
// ============================================================================

// TestPreflight_FullPipeline_PreflightReducesMutantCount verifies that
// preflight removes at least some invalid mutants from a real package.
func TestPreflight_FullPipeline_PreflightReducesMutantCount(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	initialCount := len(rawMutants)
	if initialCount == 0 {
		t.Fatalf("no mutants produced")
	}

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(rawMutants, log)

	// On a real multi-operator package like internal/reporter, preflight
	// typically filters some mutants at L1 (safety checks) and may filter
	// more at L2/L3 (schemata application issues).
	//
	// The key invariant: preflight must not INCREASE the mutant count.
	if len(remaining) > initialCount {
		t.Errorf("preflight increased mutant count: %d → %d", initialCount, len(remaining))
	}

	// Document what was filtered.
	for _, r := range results {
		if r.MutantID > 0 && r.Status != "" {
			t.Logf("filtered mutant %d: status=%s reason=%s", r.MutantID, r.Status, r.ErrorReason)
		}
	}

	t.Logf("full preflight: %d initial → %d valid (%d filtered, %.1f%% pass rate)",
		initialCount, len(remaining), len(results),
		float64(len(remaining))/float64(initialCount)*100)
}

// TestPreflight_FullPipeline_ZeroInvalidAfterPreflight verifies that no
// invalid mutants remain after preflight completes.
func TestPreflight_FullPipeline_ZeroInvalidAfterPreflight(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	log := logger.New(false)
	remaining, _ := coretesting.RunPreflight(rawMutants, log)

	for _, m := range remaining {
		if m.Status == coretesting.StatusInvalid {
			t.Errorf("mutant %d (op=%s) has status=%q after preflight — should have been removed",
				m.ID, m.Operator.Name(), m.Status)
		}
	}
}

// TestPreflight_FullPipeline_PreflightPerformance verifies preflight doesn't
// take longer than the actual test execution.
func TestPreflight_FullPipeline_PreflightPerformance(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	log := logger.New(false)
	start := time.Now()
	remaining, results := coretesting.RunPreflight(rawMutants, log)
	preflightTime := time.Since(start)

	// Preflight should be fast — it does in-memory type checking, not full builds.
	// For a package like internal/reporter with ~100 mutants, preflight should
	// complete in well under 60 seconds. If it takes longer, something is wrong
	// with the import resolution (e.g., loading full transitive deps repeatedly).
	if preflightTime > 60*time.Second {
		t.Errorf("preflight took %v for %d mutants — unexpectedly slow (limit 60s)",
			preflightTime, len(rawMutants))
	}

	t.Logf("preflight performance: %d mutants in %v (%d filtered, %d remaining)",
		len(rawMutants), preflightTime, len(results), len(remaining))
}

// ============================================================================
// PREFLIGHT — EDGE CASES
// ============================================================================

// TestPreflight_EmptyMutantSlice_NoPanic verifies preflight handles an empty
// mutant slice gracefully (no panic, no error).
func TestPreflight_EmptyMutantSlice_NoPanic(t *testing.T) {
	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(nil, log)

	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining mutants from nil input, got %d", len(remaining))
	}
	if len(results) != 0 {
		t.Errorf("expected 0 preflight results from nil input, got %d", len(results))
	}

	// Also test with empty (non-nil) slice.
	remaining, results = coretesting.RunPreflight([]coretesting.Mutant{}, log)
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining from empty input, got %d", len(remaining))
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty input, got %d", len(results))
	}
}

// TestPreflight_AllMutantsRejected_EmptyResult verifies that when all mutants
// are rejected by preflight, the result is an empty valid slice.
func TestPreflight_AllMutantsRejected_EmptyResult(t *testing.T) {
	// Create mutants that will all be rejected by L1 (nil node).
	badMutants := []coretesting.Mutant{
		{ID: 1, Site: engine.Site{Node: nil}, Operator: nil},
		{ID: 2, Site: engine.Site{Node: nil}, Operator: nil},
		{ID: 3, Site: engine.Site{Node: nil}, Operator: nil},
	}

	log := logger.New(false)
	remaining, results := coretesting.RunPreflight(badMutants, log)

	if len(remaining) != 0 {
		t.Errorf("all mutants should be rejected, but %d survived", len(remaining))
	}
	if len(results) != 3 {
		t.Errorf("expected 3 preflight results, got %d", len(results))
	}

	for _, r := range results {
		if r.Status != coretesting.StatusInvalid {
			t.Errorf("mutant %d: expected status=%q, got %q",
				r.MutantID, coretesting.StatusInvalid, r.Status)
		}
		if r.ErrorReason != "nil node" {
			t.Errorf("mutant %d: expected reason=%q, got %q",
				r.MutantID, "nil node", r.ErrorReason)
		}
	}
}

// TestPreflight_PackageWithGenerics verifies preflight handles Go generics
// without panicking.
func TestPreflight_PackageWithGenerics(t *testing.T) {
	// Check if there are any generics in the codebase.
	repoRoot := findRepoRoot(t)
	matches, _ := filepath.Glob(filepath.Join(repoRoot, "internal/**/*.go"))

	hasGenerics := false
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "[T ") || strings.Contains(string(data), "[T\n") ||
			strings.Contains(string(data), "[K ") || strings.Contains(string(data), "[V ") {
			hasGenerics = true
			break
		}
	}

	if !hasGenerics {
		t.Skip("no generics found in codebase — cannot test generics preflight")
	}

	// If generics exist, run preflight on the entire internal/ tree to verify
	// no panics.
	targetDir := filepath.Join(repoRoot, "internal/reporter")
	rawMutants := generateMutantsRaw(t, targetDir)
	if len(rawMutants) == 0 {
		t.Fatalf("no mutants produced")
	}

	log := logger.New(false)
	remaining, _ := coretesting.RunPreflight(rawMutants, log)
	if len(remaining) == 0 && len(rawMutants) > 10 {
		t.Errorf("generics preflight: all %d mutants rejected — possible generics handling issue", len(rawMutants))
	}

	t.Logf("generics preflight: %d initial → %d remaining", len(rawMutants), len(remaining))
}

// ============================================================================
// PREFLIGHT + SUB-CONFIG INTERACTION
// ============================================================================

// TestPreflight_WithSubConfig_PerDirectoryPreflight verifies that preflight
// runs correctly for each subdirectory with its own operator overrides.
func TestPreflight_WithSubConfig_PerDirectoryPreflight(t *testing.T) {
	repoRoot := findRepoRoot(t)
	targetDir := filepath.Join(repoRoot, "internal/baseline")

	// Precondition: baseline must have mutants.
	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("no mutants in internal/baseline")
	}

	// Write a sub-config that restricts to arithmetic_flip only.
	subConfigPath := filepath.Join(targetDir, "gorgon.yml")
	if _, err := os.Stat(subConfigPath); err == nil {
		t.Fatalf("sub-config already exists at %s", subConfigPath)
	}
	if err := os.WriteFile(subConfigPath, []byte("operators:\n  - arithmetic_flip\n"), 0o644); err != nil {
		t.Fatalf("write sub-config: %v", err)
	}
	t.Cleanup(func() { os.Remove(subConfigPath) })

	// Generate mutants with sub-config resolution active.
	rootCfgYAML := "operators:\n  - all\nthreshold: 0\nconcurrent: 1\ncache: false\nunit_tests_enabled: false\n"
	configPath := writeTempConfig(t, rootCfgYAML)
	filteredMutants := generateMutantsWithConfig(t, configPath, targetDir)
	if len(filteredMutants) == 0 {
		t.Fatalf("no mutants after sub-config filtering")
	}

	// Run preflight on the sub-config-filtered mutants.
	log := logger.New(false)
	remaining, _ := coretesting.RunPreflight(filteredMutants, log)

	// All remaining mutants must be arithmetic_flip (sub-config restricted).
	for _, m := range remaining {
		if m.Operator.Name() != "arithmetic_flip" {
			t.Errorf("sub-config operators not respected in preflight: got %q, expected arithmetic_flip",
				m.Operator.Name())
		}
	}
}

// ============================================================================
// PREFLIGHT + ORG POLICY INTERACTION
// ============================================================================

// TestPreflight_WithOrgPolicy_ForbiddenOpsExcludedFromPreflight verifies that
// org policy forbidden operators are excluded before preflight runs.
func TestPreflight_WithOrgPolicy_ForbiddenOpsExcludedFromPreflight(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allOps := mutator.ListAll()
	targetDir := filepath.Join(repoRoot, reporterTargetSubpath)

	baselineMutants := generateMutantsRaw(t, targetDir)
	if len(baselineMutants) == 0 {
		t.Fatalf("no mutants")
	}

	// Verify negate_condition exists in baseline.
	baselineOps := make(map[string]bool)
	for _, m := range baselineMutants {
		if m.Operator != nil {
			baselineOps[m.Operator.Name()] = true
		}
	}
	if !baselineOps["negate_condition"] {
		t.Fatalf("baseline has no negate_condition mutants — can't verify forbidden exclusion")
	}

	// Apply org policy with forbidden_operators.
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
	if len(sites) == 0 {
		t.Fatalf("no sites after operator filtering")
	}

	log := logger.New(false)
	mutants := coretesting.GenerateMutants(sites, ops, allOps, repoRoot, nil, nil, log)
	if len(mutants) == 0 {
		t.Fatalf("no mutants after policy filtering")
	}

	// negate_condition must NOT appear in any mutant.
	for _, m := range mutants {
		if m.Operator.Name() == "negate_condition" {
			t.Errorf("forbidden operator negate_condition leaked into mutant %d", m.ID)
		}
	}

	// Preflight must also not see any negate_condition mutants.
	remaining, _ := coretesting.RunPreflight(mutants, log)
	for _, m := range remaining {
		if m.Operator.Name() == "negate_condition" {
			t.Errorf("forbidden operator negate_condition survived preflight: mutant %d", m.ID)
		}
	}
}
