package testing

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aclfe/gorgon/internal/logger"
)

type PreflightResult struct {
	MutantID    int
	Status      string
	Error       error
	ErrorReason string
}

const (
	StatusValid = "valid"
)

// RunPreflight validates all mutants with three-level filtering.
//
//	Level 1 — fast static checks (nil node/file, obviously unsafe)
//	Level 2 — schemata AST integrity (can format.Node produce output?)
//	Level 3 — go/types type-check of the schemata-transformed code
//
// Level 3 catches type errors (wrong inferred types, scope leaks from :=
// wrapping, blanked imports) before the workspace build step.
func RunPreflight(mutants []Mutant, log *logger.Logger) ([]Mutant, []PreflightResult) {
	var invalid []PreflightResult

	level1Valid, level1Invalid := quickStaticFilter(mutants)
	invalid = append(invalid, level1Invalid...)
	if len(level1Valid) == 0 {
		LogPreflightResults(log, len(mutants), invalid, 0)
		return nil, invalid
	}

	level2Valid, level2Invalid := level2PackagePreflight(level1Valid)
	invalid = append(invalid, level2Invalid...)
	if len(level2Valid) == 0 {
		LogPreflightResults(log, len(mutants), invalid, 0)
		return nil, invalid
	}

	level3Valid, level3Invalid := level3TypeCheckPreflight(level2Valid, log)
	invalid = append(invalid, level3Invalid...)

	LogPreflightResults(log, len(mutants), invalid, len(level3Valid))
	return level3Valid, invalid
}

// ── Level 1 ──────────────────────────────────────────────────────────────────

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

// ── Level 2 ──────────────────────────────────────────────────────────────────

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
		groups[key] = append(groups[key], *m)
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
		mutant := mutants[j]

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

// ── Level 3 ──────────────────────────────────────────────────────────────────

// level3TypeCheckPreflight groups mutants by source file and type-checks each
// mutant's schemata-transformed AST using go/types.
//
// It computes a baseline of type errors present in the unmodified file first
// (using a lenient importer that stubs unresolvable packages). Only errors that
// are NEW relative to the baseline are attributed to the mutation. This prevents
// third-party import resolution failures from generating false positives.
func level3TypeCheckPreflight(mutants []Mutant, log *logger.Logger) ([]Mutant, []PreflightResult) {
	// Group by source file.
	groups := make(map[string][]Mutant)
	for i := range mutants {
		if mutants[i].Site.File != nil {
			key := mutants[i].Site.File.Name()
			groups[key] = append(groups[key], mutants[i])
		}
	}

	// Process files with bounded concurrency.
	// go/types.Check is CPU-heavy and allocates significantly per call.
	// 2 concurrent files is safe even on constrained CI environments.
	type result struct {
		valid   []Mutant
		invalid []PreflightResult
	}
	resultCh := make(chan result, len(groups))
	sem := make(chan struct{}, 2)

	var wg sync.WaitGroup
	for filePath, fileMutants := range groups {
		filePath := filePath
		fileMutants := fileMutants
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			v, inv := typeCheckFileGroup(filePath, fileMutants, log)
			resultCh <- result{v, inv}
		}()
	}

	wg.Wait()
	close(resultCh)

	var valid []Mutant
	var invalid []PreflightResult
	for r := range resultCh {
		valid = append(valid, r.valid...)
		invalid = append(invalid, r.invalid...)
	}
	return valid, invalid
}


// schemataHelperSrc is injected into every package during the real mutation run.
// Including it here lets go/types resolve the activeMutantID identifier that
// the schemata transformation introduces.
const schemataHelperSrc = `package %s

import (
	"os"
	"strconv"
)

var activeMutantID int

func init() {
	if idStr := os.Getenv("GORGON_MUTANT_ID"); idStr != "" {
		activeMutantID, _ = strconv.Atoi(idStr)
	}
}
`

type siblingFile struct {
	path string
	src  []byte
}

func typeCheckFileMutants(filePath string, mutants []Mutant, log *logger.Logger) ([]Mutant, []PreflightResult) {
	pkgDir := filepath.Dir(filePath)

	src, err := os.ReadFile(filePath)
	if err != nil {
		log.Debug("[PREFLIGHT L3] Cannot read %s — skipping type-check for %d mutant(s)", filePath, len(mutants))
		return mutants, nil
	}

	// Load sibling file bytes once for the whole group.
	// We can't share parsed *ast.File nodes across type-check calls because each
	// call needs its own token.FileSet, so we store raw bytes and re-parse cheaply.
	siblings := loadSiblingFiles(pkgDir, filePath)
	pkgName := parsePackageName(src, filePath)
	helper := fmt.Sprintf(schemataHelperSrc, pkgName)

	// One lenient importer shared across all mutants in this file.
	// It caches resolved packages, so each package is fetched at most once.
	imp := &lenientImporter{base: importer.Default()}

	// Baseline: how many times does each error message appear in the ORIGINAL
	// (untransformed) file? These are errors from lenient import stubs, not from
	// mutations — we subtract them so they don't generate false positives.
	baseline := computeBaselineErrors(filePath, src, siblings, helper, pkgDir, imp)
	log.Debug("[PREFLIGHT L3] %s: %d baseline error message(s)", filepath.Base(filePath), len(baseline))

	var valid []Mutant
	var invalid []PreflightResult

	for _, mutant := range mutants {
		mc := mutant

		// Apply schemata in-memory (no disk I/O).
		fset := token.NewFileSet()
		mutatedAST, err := ApplySchemataInMemory(src, filePath, fset, []*Mutant{&mc})
		if err != nil {
			invalid = append(invalid, PreflightResult{
				MutantID:    mc.ID,
				Status:      StatusCompileError,
				ErrorReason: "in-memory schemata: " + err.Error(),
			})
			continue
		}

		// Build the file set for this type-check call:
		// mutated file + all siblings (with their original source) + helper.
		allFiles := []*ast.File{mutatedAST}
		for _, sib := range siblings {
			if f, pErr := parser.ParseFile(fset, sib.path, sib.src, 0); pErr == nil {
				allFiles = append(allFiles, f)
			}
		}
		if hf, pErr := parser.ParseFile(fset, "gorgon_schemata.go", helper, 0); pErr == nil {
			allFiles = append(allFiles, hf)
		}

		// Per-mutant copy of the baseline budget so concurrent callers don't race.
		// (This function is not currently called concurrently, but it's cheap insurance.)
		budget := make(map[string]int, len(baseline))
		for k, v := range baseline {
			budget[k] = v
		}

		var newErrors []string
		conf := &types.Config{
			Importer: imp,
			Error: func(e error) {
				msg := typeErrorMessage(e.Error())
				if budget[msg] > 0 {
					budget[msg]-- // absorbed by baseline
				} else {
					newErrors = append(newErrors, e.Error())
				}
			},
		}
		conf.Check(pkgDir, fset, allFiles, nil)

		if len(newErrors) > 0 {
			log.Debug("[PREFLIGHT L3] Mutant #%d filtered — %s", mc.ID, newErrors[0])
			invalid = append(invalid, PreflightResult{
				MutantID:    mc.ID,
				Status:      StatusCompileError,
				ErrorReason: "type check: " + newErrors[0],
			})
		} else {
			valid = append(valid, mc)
		}
	}

	return valid, invalid
}

// computeBaselineErrors returns a multiset (message → count) of type errors
// present in the ORIGINAL, untransformed file. Used to subtract pre-existing
// errors (typically from lenient import stubs) from mutation-introduced errors.
func computeBaselineErrors(filePath string, src []byte, siblings []siblingFile, helper, pkgDir string, imp types.Importer) map[string]int {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, filePath, src, 0)
	if err != nil {
		return nil
	}

	allFiles := []*ast.File{f}
	for _, sib := range siblings {
		if sf, err := parser.ParseFile(fset, sib.path, sib.src, 0); err == nil {
			allFiles = append(allFiles, sf)
		}
	}
	if hf, err := parser.ParseFile(fset, "gorgon_schemata.go", helper, 0); err == nil {
		allFiles = append(allFiles, hf)
	}

	counts := make(map[string]int)
	conf := &types.Config{
		Importer: imp,
		Error: func(e error) {
			counts[typeErrorMessage(e.Error())]++
		},
	}
	conf.Check(pkgDir, fset, allFiles, nil)
	return counts
}

// typeErrorMessage strips the "file:line:col: " prefix from a go/types error
// so errors from different positions but the same root cause compare equal.
//
//	"/abs/path/file.go:42:10: cannot use string as int value"
//	→ "cannot use string as int value"
func typeErrorMessage(s string) string {
	if idx := strings.Index(s, ": "); idx >= 0 {
		return s[idx+2:]
	}
	return s
}

func loadSiblingFiles(pkgDir, excludePath string) []siblingFile {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	var out []siblingFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		p := filepath.Join(pkgDir, name)
		if p == excludePath {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out = append(out, siblingFile{p, data})
	}
	return out
}

func parsePackageName(src []byte, filePath string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, 0)
	if err != nil || f.Name == nil {
		return filepath.Base(filepath.Dir(filePath))
	}
	return f.Name.Name
}

// ── Lenient importer ─────────────────────────────────────────────────────────

// lenientImporter wraps the default importer and returns an empty stub package
// for any path that cannot be resolved, so type-checking continues instead of
// aborting on the first missing dependency.
//
// Errors caused by stubs (e.g. "undefined: yaml.Unmarshal") appear in both the
// baseline and the mutated files and are therefore cancelled out by the budget.
type lenientImporter struct {
	base  types.Importer
	mu    sync.Mutex
	cache map[string]*types.Package
}

func (l *lenientImporter) Import(path string) (*types.Package, error) {
	l.mu.Lock()
	if l.cache == nil {
		l.cache = make(map[string]*types.Package)
	}
	if cached, ok := l.cache[path]; ok {
		l.mu.Unlock()
		return cached, nil
	}
	l.mu.Unlock()

	pkg, err := l.base.Import(path)
	if err != nil {
		// Stub: an empty package with no exported names.
		// References to its symbols produce "undefined" errors, which the
		// baseline multiset will absorb if they pre-exist in the original.
		pkg = types.NewPackage(path, filepath.Base(path))
	}

	l.mu.Lock()
	l.cache[path] = pkg
	l.mu.Unlock()
	return pkg, nil // always nil error — never abort type-checking
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
	return false
}

// LogPreflightResults prints a summary of filtered mutants.
func LogPreflightResults(log *logger.Logger, totalMutants int, results []PreflightResult, validCount int) {
	level1, level2, level3 := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case StatusInvalid:
			level1++
		case StatusCompileError:
			// We can't distinguish L2 vs L3 from Status alone without an extra field.
			// For now, bucket them together as "compile-time filtered".
			level2++
		}
	}
	// Note: level2 here actually includes both L2 and L3 rejections.
	_ = level3
	log.Print("[PREFLIGHT] L1 filtered %d | L2+L3 filtered %d | Remaining: %d (of %d)",
		level1, level2, validCount, totalMutants)
}

func typeCheckFileGroup(filePath string, mutants []Mutant, log *logger.Logger) ([]Mutant, []PreflightResult) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		// Can't read file — pass all mutants through; the build will catch real errors.
		log.Debug("[PREFLIGHT L3] Cannot read %s — skipping type-check", filePath)
		return mutants, nil
	}

	pkgDir := filepath.Dir(filePath)
	siblings := loadSiblingFiles(pkgDir, filePath)
	pkgName := parsePackageName(src, filePath)
	helper := fmt.Sprintf(schemataHelperSrc, pkgName)
	imp := &lenientImporter{base: importer.Default()}

	// Baseline errors from the unmodified file — absorbed before attribution.
	baseline := computeBaselineErrors(filePath, src, siblings, helper, pkgDir, imp)

	for msg := range baseline {
		if !isImportStubError(msg) {
			log.Debug("[PREFLIGHT L3] %s: real baseline type error detected (%q) — rejecting all %d mutant(s)",
				filepath.Base(filePath), msg, len(mutants))
			_, invalid := makeAllInvalid(mutants, "baseline type error: "+msg)
			return nil, invalid
		}
	}

	// Convert to pointers for ApplySchemataInMemory.
	mutPtrs := make([]*Mutant, len(mutants))
	for i := range mutants {
		mutPtrs[i] = &mutants[i]
	}

	// One type-check for all mutants combined.
	errs, panicked := runTypeCheck(filePath, src, mutPtrs, siblings, helper, pkgDir, imp, baseline)
	if panicked {
		// Don't pass through — bisect to find the individual bad mutants.
		log.Debug("[PREFLIGHT L3] %s: go/types panicked on group of %d mutants, bisecting to find bad ones",
			filepath.Base(filePath), len(mutants))
		return bisectMutants(filePath, src, mutants, siblings, helper, pkgDir, imp, baseline, log)
	}

	if len(errs) > 0 {
		// Errors found — bisect to find which mutants are responsible.
		// This only happens for files that actually produce invalid mutations.
		log.Debug("[PREFLIGHT L3] %s: %d type errors found, bisecting %d mutants",
			filepath.Base(filePath), len(errs), len(mutants))
		return bisectMutants(filePath, src, mutants, siblings, helper, pkgDir, imp, baseline, log)
	}

	return mutants, nil
}

func isImportStubError(msg string) bool {
	return strings.HasPrefix(msg, "undefined:") ||
		strings.Contains(msg, "has no field or method") ||
		strings.Contains(msg, "cannot refer to unexported")
}

func runTypeCheck(filePath string, src []byte, mutPtrs []*Mutant, siblings []siblingFile, helper, pkgDir string, imp types.Importer, baseline map[string]int) (newErrors []string, panicked bool) {
    fset := token.NewFileSet()
    mutatedAST, err := ApplySchemataInMemory(src, filePath, fset, mutPtrs)
    if err != nil {
        return []string{"schemata: " + err.Error()}, false
    }

    allFiles := []*ast.File{mutatedAST}
    for _, sib := range siblings {
        if f, err := parser.ParseFile(fset, sib.path, sib.src, 0); err == nil {
            allFiles = append(allFiles, f)
        }
    }
    if hf, err := parser.ParseFile(fset, "gorgon_schemata.go", helper, 0); err == nil {
        allFiles = append(allFiles, hf)
    }

    budget := make(map[string]int, len(baseline))
    for k, v := range baseline {
        budget[k] = v
    }

    defer func() {
        if r := recover(); r != nil {
            // go/types panicked formatting an error about a schemata-generated node.
            // We can't determine which mutants are bad, so pass them all through.
            // The real build step will catch any actual compile errors.
            newErrors = nil
            panicked = true
        }
    }()

    conf := &types.Config{
        Importer: imp,
        Error: func(e error) {
            msg := typeErrorMessage(e.Error())
            if budget[msg] > 0 {
                budget[msg]--
            } else {
                newErrors = append(newErrors, e.Error())
            }
        },
    }
    conf.Check(pkgDir, fset, allFiles, nil)
    return newErrors, false
}



func bisectMutants(filePath string, src []byte, mutants []Mutant, siblings []siblingFile, helper, pkgDir string, imp types.Importer, baseline map[string]int, log *logger.Logger) ([]Mutant, []PreflightResult) {
	if len(mutants) == 0 {
		return nil, nil
	}

	if len(mutants) == 1 {
		// Base case: single mutant — either it's bad or it's fine.
		mutPtrs := []*Mutant{&mutants[0]}
		errs, panicked := runTypeCheck(filePath, src, mutPtrs, siblings, helper, pkgDir, imp, baseline)
		if panicked {
			// A panic means go/types cannot handle this schemata-generated AST.
			// Reject instead of passing through.
			log.Debug("[PREFLIGHT L3] Mutant #%d rejected: go/types panicked on schemata AST (likely malformed IIFE type)", mutants[0].ID)
			return nil, []PreflightResult{{
				MutantID:    mutants[0].ID,
				Status:      StatusCompileError,
				ErrorReason: "type check: go/types panicked on schemata-generated AST (malformed IIFE return type)",
			}}
		}
		if len(errs) > 0 {
			log.Debug("[PREFLIGHT L3] Mutant #%d rejected: %s", mutants[0].ID, errs[0])
			return nil, []PreflightResult{{
				MutantID:    mutants[0].ID,
				Status:      StatusCompileError,
				ErrorReason: "type check: " + errs[0],
			}}
		}
		return mutants, nil
	}

	mid := len(mutants) / 2
	left := mutants[:mid]
	right := mutants[mid:]

	// Check left half.
	leftPtrs := make([]*Mutant, len(left))
	for i := range left {
		leftPtrs[i] = &left[i]
	}
	leftErrs, leftPanicked := runTypeCheck(filePath, src, leftPtrs, siblings, helper, pkgDir, imp, baseline)

	// Check right half.
	rightPtrs := make([]*Mutant, len(right))
	for i := range right {
		rightPtrs[i] = &right[i]
	}
	rightErrs, rightPanicked := runTypeCheck(filePath, src, rightPtrs, siblings, helper, pkgDir, imp, baseline)

	var valid []Mutant
	var invalid []PreflightResult

	if !leftPanicked && len(leftErrs) > 0 {
		v, inv := bisectMutants(filePath, src, left, siblings, helper, pkgDir, imp, baseline, log)
		valid = append(valid, v...)
		invalid = append(invalid, inv...)
	} else {
		valid = append(valid, left...)
	}

	if !rightPanicked && len(rightErrs) > 0 {
		v, inv := bisectMutants(filePath, src, right, siblings, helper, pkgDir, imp, baseline, log)
		valid = append(valid, v...)
		invalid = append(invalid, inv...)
	} else {
		valid = append(valid, right...)
	}

	return valid, invalid
}
