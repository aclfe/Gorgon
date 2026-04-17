package testing

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"

	"github.com/aclfe/gorgon/internal/logger"
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
		if m.Site.File == nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    m.ID,
				Status:      StatusInvalid,
				ErrorReason: "nil file",
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

// level2PackagePreflight does the AST-integrity check using schemata.
// It groups mutants by file, applies schemata once per file, and validates
// that the resulting AST is structurally sound.
// Mutants with nil File are now explicitly rejected and counted in Level2
// (no more silent drops that made the numbers not add up).
func level2PackagePreflight(mutants []Mutant) ([]Mutant, []PreflightResult) {
	if len(mutants) == 0 {
		return nil, nil
	}

	groups := make(map[string][]Mutant)
	var invalid []PreflightResult

	for i := range mutants {
		m := &mutants[i]
		if m.Site.File == nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    m.ID,
				Status:      StatusInvalid,
				ErrorReason: "nil file",
			})
			m.Status = StatusInvalid
			continue
		}

		key := m.Site.File.Name()
		groups[key] = append(groups[key], *m) // copy so we don't mutate range var
	}

	var valid []Mutant

	for filePath, fileMutants := range groups {
		fileValid, fileInvalid := checkFileWithSchemata(filePath, fileMutants)
		valid = append(valid, fileValid...)
		invalid = append(invalid, fileInvalid...)
	}

	return valid, invalid
}

func checkFileWithSchemata(filePath string, mutants []Mutant) ([]Mutant, []PreflightResult) {
	if len(mutants) == 0 {
		return nil, nil
	}

	src, err := os.ReadFile(filePath)
	if err != nil {
		return makeAllInvalid(mutants, fmt.Sprintf("cannot read source file: %v", err))
	}

	var valid []Mutant
	var invalid []PreflightResult

	for j := range mutants {
		mutant := mutants[j] // struct copy (safe for &mutant pointer)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
		if err != nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    mutant.ID,
				Status:      StatusCompileError,
				ErrorReason: fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		tmpf, err := os.CreateTemp("", "gorgon-preflight-*.go")
		if err != nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    mutant.ID,
				Status:      StatusCompileError,
				ErrorReason: fmt.Sprintf("cannot create temp file for preflight: %v", err),
			})
			continue
		}
		tmpPath := tmpf.Name()
		tmpf.Close()

		mutantsPtr := []*Mutant{&mutant}
		posMap, schemataErr := ApplySchemataToAST(file, fset, tmpPath, src, mutantsPtr)

		_ = os.Remove(tmpPath)

		if schemataErr != nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    mutant.ID,
				Status:      StatusCompileError,
				ErrorReason: fmt.Sprintf("schemata apply failed: %v", schemataErr),
			})
			continue
		}

		if posMap == nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    mutant.ID,
				Status:      StatusCompileError,
				ErrorReason: "schemata produced an un-formattable AST",
			})
			continue
		}

		if pm, ok := posMap[mutant.ID]; ok {
			mutant.TempLine = pm.TempLine
			mutant.TempCol = pm.TempCol
		}

		valid = append(valid, mutant)
	}

	return valid, invalid
}

// Helper to mark every mutant in a group as invalid
func makeAllInvalid(mutants []Mutant, reason string) ([]Mutant, []PreflightResult) {
	invalid := make([]PreflightResult, len(mutants))
	for i := range mutants {
		invalid[i] = PreflightResult{
			MutantID:    mutants[i].ID,
			Status:      StatusCompileError,
			ErrorReason: reason,
		}
		mutants[i].Status = StatusCompileError
	}
	return nil, invalid
}

func isObviouslyUnsafeMutation(m *Mutant) bool {
	// Add cheap static rules here
	// For now, return false - all mutations pass Level 1
	return false
}

// LogPreflightResults prints a summary of filtered mutants.
// totalMutants is len(mutants) after GenerateMutants — NOT the site count.
// results contains Level1 (static) and Level2 (schemata) invalid mutants.
// validCount is the number of mutants that passed all preflight checks.
// Invariant: Level1 + Level2 + validCount == totalMutants
func LogPreflightResults(log *logger.Logger, totalMutants int, results []PreflightResult, validCount int) {
	level1 := 0
	level2 := 0
	for _, r := range results {
		if r.Status == StatusInvalid {
			level1++
		} else if r.Status == StatusCompileError {
			level2++
		}
	}

	log.Print("[PREFLIGHT] Level1 filtered %d | Level2 filtered %d | Remaining valid: %d (of %d mutants)",
		level1, level2, validCount, totalMutants)
}
