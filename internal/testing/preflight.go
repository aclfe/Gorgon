package testing

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
)

type PreflightResult struct {
	MutantID    int
	Status      string
	Error       error
	ErrorReason string
}

const (
	StatusValid        = "valid"
	StatusInvalid      = "invalid"
	StatusCompileError = "error"
)

// Level 1: Very fast static checks (no build)
func quickStaticFilter(mutants []Mutant) ([]Mutant, []PreflightResult) {
	valid := make([]Mutant, 0, len(mutants))
	var invalid []PreflightResult

	for i := range mutants {
		m := &mutants[i]
		if m.Site.Node == nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    m.ID,
				Status:      StatusInvalid,
				ErrorReason: "nil node",
			})
			m.Status = StatusInvalid
			continue
		}

		// Very cheap static checks
		if isObviouslyUnsafeMutation(m) {
			invalid = append(invalid, PreflightResult{
				MutantID:    m.ID,
				Status:      StatusInvalid,
				ErrorReason: "obviously unsafe mutation",
			})
			m.Status = StatusInvalid
			continue
		}

		valid = append(valid, *m)
	}

	return valid, invalid
}

// Level 2: Accurate per-package schemata AST check.
// Groups mutants by file, applies schemata in-memory, and validates that the
// resulting AST can be formatted cleanly. This replaces the old canaryBuild
// (which tried to compile in an isolated fake module — always broken for
// packages with real imports).
func PreflightFilterWithResults(mutants []Mutant) ([]Mutant, []PreflightResult) {
	if len(mutants) == 0 {
		return nil, nil
	}

	// Level 1 - fast static filter
	validAfterLevel1, level1Invalid := quickStaticFilter(mutants)

	// Level 2 - group by file/package and do AST integrity check
	validFinal, level2Invalid := level2PackagePreflight(validAfterLevel1)

	allInvalid := append(level1Invalid, level2Invalid...)
	return validFinal, allInvalid
}

// level2PackagePreflight does the AST-integrity check using schemata.
// It groups mutants by file, applies schemata once per file, and validates
// that the resulting AST is structurally sound.
func level2PackagePreflight(mutants []Mutant) ([]Mutant, []PreflightResult) {
	if len(mutants) == 0 {
		return nil, nil
	}

	// Group mutants by source file (schemata is applied per file)
	groups := make(map[string][]Mutant)
	for _, m := range mutants {
		if m.Site.File == nil {
			continue
		}
		key := m.Site.File.Name()
		groups[key] = append(groups[key], m)
	}

	var valid []Mutant
	var invalid []PreflightResult

	for filePath, fileMutants := range groups {
		fileValid, fileInvalid := checkFileWithSchemata(filePath, fileMutants)
		valid = append(valid, fileValid...)
		invalid = append(invalid, fileInvalid...)
	}

	return valid, invalid
}

// checkFileWithSchemata applies schemata to one file and validates the
// resulting AST using format.Node (no subprocess, no isolated module).
// Any mutation that makes the AST un-formattable is marked invalid.
// Real type errors are caught downstream by compileWithSurgicalRetry.
func checkFileWithSchemata(filePath string, mutants []Mutant) ([]Mutant, []PreflightResult) {
	if len(mutants) == 0 {
		return nil, nil
	}

	// 1. Parse the original source
	fset := token.NewFileSet()
	src, err := os.ReadFile(filePath)
	if err != nil {
		return makeAllInvalid(mutants, fmt.Sprintf("cannot read source file: %v", err))
	}

	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return makeAllInvalid(mutants, fmt.Sprintf("parse error: %v", err))
	}

	// Convert to pointer slice for ApplySchemataToAST
	mutantsPtr := make([]*Mutant, len(mutants))
	for i := range mutants {
		mutantsPtr[i] = &mutants[i]
	}

	// 2. Apply schemata — this modifies `file` in place and writes to filePath.
	//    We restore the original source in all cases immediately after.
	posMap, schemataErr := ApplySchemataToAST(file, fset, filePath, src, mutantsPtr)

	// Always restore original source — schemata writes to disk as a side-effect.
	if len(src) > 0 {
		_ = os.WriteFile(filePath, src, 0644)
	}

	if schemataErr != nil {
		return makeAllInvalid(mutants, fmt.Sprintf("schemata apply failed: %v", schemataErr))
	}

	// ApplySchemataToAST returns nil posMap (and nil error) when format.Node
	// fails on the modified AST — treat this as an AST integrity failure.
	if posMap == nil {
		return makeAllInvalid(mutants, "schemata produced an un-formattable AST")
	}

	// 3. Update TempLine/TempCol on mutants from position map
	for i := range mutants {
		if pm, ok := posMap[mutants[i].ID]; ok {
			mutants[i].TempLine = pm.TempLine
			mutants[i].TempCol = pm.TempCol
		}
	}

	// All mutants passed the AST integrity check — mark as valid for preflight.
	return mutants, nil
}

// Helper to mark every mutant in a group as invalid
func makeAllInvalid(mutants []Mutant, reason string) ([]Mutant, []PreflightResult) {
	invalid := make([]PreflightResult, len(mutants))
	for i, m := range mutants {
		invalid[i] = PreflightResult{
			MutantID:    m.ID,
			Status:      StatusCompileError,
			ErrorReason: reason,
		}
		m.Status = StatusCompileError
	}
	return nil, invalid
}

// Lightweight version of attribute errors for preflight
func attributeCompileErrorsForPreflight(filePath string, mutants []Mutant, compilerOutput string) map[int]bool {
	bad := make(map[int]bool)

	errors := ParseCompilerErrors(compilerOutput)
	if len(errors) == 0 {
		// If we couldn't parse errors, mark all as bad (safe fallback)
		for _, m := range mutants {
			bad[m.ID] = true
		}
		return bad
	}

	// Build position map for mutants
	type pos struct {
		line int
		col  int
		id   int
	}
	var positions []pos
	for _, m := range mutants {
		positions = append(positions, pos{line: m.TempLine, col: m.TempCol, id: m.ID})
	}

	// For each error, find the closest mutant
	for _, err := range errors {
		bestID := -1
		bestDist := 1000000

		for _, p := range positions {
			if err.Line == p.line {
				dist := absInt(err.Col - p.col)
				if dist < bestDist {
					bestDist = dist
					bestID = p.id
				}
			}
		}

		// If error is within reasonable distance, mark that mutant
		if bestID >= 0 && bestDist <= 5 {
			bad[bestID] = true
		}
	}

	// If no errors could be attributed, mark all as bad (safe fallback)
	if len(bad) == 0 {
		for _, m := range mutants {
			bad[m.ID] = true
		}
	}

	return bad
}

func isObviouslyUnsafeMutation(m *Mutant) bool {
	// Add cheap static rules here
	// For now, return false - all mutations pass Level 1
	return false
}

// LogPreflightResults prints a summary of filtered mutants to stderr.
// validCount is the number of mutants that passed preflight and remain.
func LogPreflightResults(results []PreflightResult, validCount int) {
	if len(results) == 0 {
		return
	}

	level1 := 0
	level2 := 0
	for _, r := range results {
		if r.Status == StatusInvalid {
			level1++
		} else if r.Status == StatusCompileError {
			level2++
		}
	}

	fmt.Fprintf(os.Stderr, "[PREFLIGHT] Level1 filtered %d | Level2 filtered %d | Remaining valid: %d\n",
		level1, level2, validCount)
}
