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
		// Skip preflight for files with too many mutants to avoid OOM
		// They'll be validated during actual compilation instead
		if len(fileMutants) > 50 {
			valid = append(valid, fileMutants...)
			continue
		}
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

	// Try all mutants together first (fast path)
	valid, invalid, ok := tryApplySchemata(filePath, src, mutants)
	if ok {
		return valid, invalid
	}

	// Combined application failed — validate individually to isolate bad mutants
	var allValid []Mutant
	var allInvalid []PreflightResult
	for i := range mutants {
		v, inv, _ := tryApplySchemata(filePath, src, mutants[i:i+1])
		allValid = append(allValid, v...)
		allInvalid = append(allInvalid, inv...)
	}
	return allValid, allInvalid
}

func tryApplySchemata(filePath string, src []byte, mutants []Mutant) ([]Mutant, []PreflightResult, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, makeAllInvalidWith(mutants, fmt.Sprintf("parse error: %v", err)), false
	}

	tmpf, err := os.CreateTemp("", "gorgon-preflight-*.go")
	if err != nil {
		return nil, makeAllInvalidWith(mutants, fmt.Sprintf("cannot create temp file: %v", err)), false
	}
	tmpPath := tmpf.Name()
	tmpf.Close()
	defer os.Remove(tmpPath)

	mutantsPtr := make([]*Mutant, len(mutants))
	for i := range mutants {
		mutantsPtr[i] = &mutants[i]
	}

	posMap, schemataErr := ApplySchemataToAST(file, fset, tmpPath, src, mutantsPtr)
	if schemataErr != nil || posMap == nil {
		return nil, nil, false
	}

	valid := make([]Mutant, 0, len(mutants))
	for i := range mutants {
		if pm, ok := posMap[mutants[i].ID]; ok {
			mutants[i].TempLine = pm.TempLine
			mutants[i].TempCol = pm.TempCol
		}
		valid = append(valid, mutants[i])
	}
	return valid, nil, true
}

func makeAllInvalidWith(mutants []Mutant, reason string) []PreflightResult {
	inv := make([]PreflightResult, len(mutants))
	for i := range mutants {
		inv[i] = PreflightResult{MutantID: mutants[i].ID, Status: StatusCompileError, ErrorReason: reason}
		mutants[i].Status = StatusCompileError
	}
	return inv
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
