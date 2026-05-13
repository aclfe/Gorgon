//go:build integration
// +build integration

package integration

import (
	"testing"

	coretesting "github.com/aclfe/gorgon/internal/core"
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
func TestWorkflow_PreflightCatchesBaselineErrors(t *testing.T) {
	t.Skip("TODO: create a source file with a deliberate type error; " +
		"run preflight on it; assert the type error is detected and the " +
		"file is rejected before schemata is applied")
}

// TestWorkflow_AllPreflightPhasesWork verifies that all phases are able to
// filter mutations properly and not just be dummy.
func TestWorkflow_AllPreflightPhasesWork(t *testing.T) {
	t.Skip("TODO: run the full preflight (levels 1, 2, 3) on a real package; " +
		"assert mutants are filtered at each level; assert the final mutant count " +
		"is less than or equal to the initial count (some mutants removed)")
}

// ============================================================================
// PREFLIGHT — LEVEL 1 (STATIC CHECKS)
// ============================================================================

// TestPreflight_Level1_OperatorSpecificPreflight verifies that each operator's
// preflight hook runs and can reject mutants before AST transformation.
func TestPreflight_Level1_OperatorSpecificPreflight(t *testing.T) {
	t.Skip("TODO: run preflight level 1 on mutants from multiple operators; " +
		"assert each operator's preflight hook is called; " +
		"assert mutants rejected by preflight have status set appropriately")
}

// TestPreflight_Level1_NoOperatorsWithPreflight_BypassesLevel1 verifies that
// when no operators have preflight hooks, level 1 is a no-op.
func TestPreflight_Level1_NoOperatorsWithPreflight_BypassesLevel1(t *testing.T) {
	t.Skip("TODO: list all operators; for those with preflight hooks, assert " +
		"they are called; for those without, assert level 1 is skipped")
}

// TestPreflight_Level1_InvalidMutant_RemovedFromSlice verifies that mutants
// rejected at level 1 are removed from the mutant slice before level 2.
func TestPreflight_Level1_InvalidMutant_RemovedFromSlice(t *testing.T) {
	t.Skip("TODO: create a mutant that fails level 1 preflight; run preflight; " +
		"assert the mutant is removed from the slice and its status is set")
}

// ============================================================================
// PREFLIGHT — LEVEL 2 (IN-MEMORY SCHEMATA + TYPE CHECK)
// ============================================================================

// TestPreflight_Level2_InMemorySchemata_CompilesSuccessfully verifies that
// ApplySchemataInMemory produces a valid AST that passes type checking.
func TestPreflight_Level2_InMemorySchemata_CompilesSuccessfully(t *testing.T) {
	_ = coretesting.ApplySchemataInMemory
	t.Skip("TODO: generate mutants for a file; call ApplySchemataInMemory; " +
		"assert the returned *ast.File is non-nil; type-check it; " +
		"assert no type errors")
}

// TestPreflight_Level2_TypeError_Detected verifies that schemata-induced type
// errors are caught by the level 2 type check.
func TestPreflight_Level2_TypeError_Detected(t *testing.T) {
	t.Skip("TODO: create a mutant that would produce a type error (e.g., returning " +
		"nil for a non-nilable type incorrectly); run preflight level 2; " +
		"assert the type error is detected and the mutant is rejected")
}

// TestPreflight_Level2_MultipleFilesInPackage verifies level 2 works when a
// package has multiple files (type checking must resolve cross-file references).
func TestPreflight_Level2_MultipleFilesInPackage(t *testing.T) {
	t.Skip("TODO: target a package with 2+ .go files; generate mutants across all " +
		"files; run preflight level 2; assert type checking across files works")
}

// TestPreflight_Level2_CrossPackageReferences verifies level 2 handles
// references to types/functions from imported packages.
func TestPreflight_Level2_CrossPackageReferences(t *testing.T) {
	t.Skip("TODO: target a file that imports another package; generate mutants; " +
		"run level 2; assert type checking resolves external types correctly")
}

// ============================================================================
// PREFLIGHT — LEVEL 3 (FILE GROUP TYPE CHECK)
// ============================================================================

// TestPreflight_Level3_TypeCheckFileGroup verifies that typeCheckFileGroup
// catches errors that span multiple files in the same package.
func TestPreflight_Level3_TypeCheckFileGroup(t *testing.T) {
	t.Skip("TODO: create mutants in two files of the same package; " +
		"run typeCheckFileGroup; assert errors that span files are caught")
}

// TestPreflight_Level3_SingleFilePackage verifies level 3 works for a
// single-file package.
func TestPreflight_Level3_SingleFilePackage(t *testing.T) {
	t.Skip("TODO: target a single-file package; run typeCheckFileGroup; " +
		"assert it completes without error (single file is valid input)")
}

// ============================================================================
// PREFLIGHT — FULL PIPELINE
// ============================================================================

// TestPreflight_FullPipeline_PreflightReducesMutantCount verifies that
// preflight removes at least some invalid mutants from a real package.
func TestPreflight_FullPipeline_PreflightReducesMutantCount(t *testing.T) {
	t.Skip("TODO: run GenerateAndRunSchemata with preflight enabled; " +
		"count mutants before and after preflight; assert some are removed; " +
		"assert the removed mutants have status \"invalid\" or \"error\"")
}

// TestPreflight_FullPipeline_ZeroInvalidAfterPreflight verifies that no
// invalid mutants remain after preflight completes.
func TestPreflight_FullPipeline_ZeroInvalidAfterPreflight(t *testing.T) {
	t.Skip("TODO: run pipeline with preflight; check all remaining mutants; " +
		"assert none have status \"invalid\" (all invalid ones were filtered)")
}

// TestPreflight_FullPipeline_PreflightPerformance verifies preflight doesn't
// take longer than the actual test execution.
func TestPreflight_FullPipeline_PreflightPerformance(t *testing.T) {
	t.Skip("TODO: time preflight vs test execution; assert preflight time < " +
		"2x test execution time (preflight should be fast since it avoids full builds)")
}

// ============================================================================
// PREFLIGHT — EDGE CASES
// ============================================================================

// TestPreflight_EmptyMutantSlice_NoPanic verifies preflight handles an empty
// mutant slice gracefully (no panic, no error).
func TestPreflight_EmptyMutantSlice_NoPanic(t *testing.T) {
	t.Skip("TODO: call RunPreflight with an empty mutant slice; " +
		"assert no panic; assert returned slice is also empty")
}

// TestPreflight_AllMutantsRejected_EmptyResult verifies that when all mutants
// are rejected by preflight, the result is an empty slice.
func TestPreflight_AllMutantsRejected_EmptyResult(t *testing.T) {
	t.Skip("TODO: create mutants that all fail preflight (e.g., all type errors); " +
		"run preflight; assert the returned slice is empty")
}

// TestPreflight_PackageWithGenerics verifies preflight handles Go generics.
func TestPreflight_PackageWithGenerics(t *testing.T) {
	t.Skip("TODO: create a file with generic functions; generate mutants; " +
		"run preflight; assert type checking works correctly with type parameters")
}

// ============================================================================
// PREFLIGHT + SUB-CONFIG INTERACTION
// ============================================================================

// TestPreflight_WithSubConfig_PerDirectoryPreflight verifies that preflight
// runs correctly for each subdirectory with its own operator overrides.
func TestPreflight_WithSubConfig_PerDirectoryPreflight(t *testing.T) {
	t.Skip("TODO: set up sub-configs with different operators per dir; " +
		"run preflight targeting all dirs; assert each dir's preflight uses " +
		"the correct effective operators from sub-config resolution")
}

// ============================================================================
// PREFLIGHT + ORG POLICY INTERACTION
// ============================================================================

// TestPreflight_WithOrgPolicy_ForbiddenOpsExcludedFromPreflight verifies that
// org policy forbidden operators are excluded before preflight runs.
func TestPreflight_WithOrgPolicy_ForbiddenOpsExcludedFromPreflight(t *testing.T) {
	t.Skip("TODO: apply org policy with forbidden_operators; run preflight; " +
		"assert forbidden operators are not in the mutant list during preflight")
}
