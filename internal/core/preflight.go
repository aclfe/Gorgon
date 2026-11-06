package testing

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/mutator"
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
	return RunPreflightWithTags(mutants, nil, log)
}

func RunPreflightWithTags(mutants []Mutant, buildTags []string, log *logger.Logger) ([]Mutant, []PreflightResult) {
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

	level3Valid, level3Invalid := level3TypeCheckPreflight(level2Valid, buildTags, log)
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

	ptrs := make([]*Mutant, len(mutants))
	for i := range mutants {
		ptrs[i] = &mutants[i]
	}

	var valid []Mutant
	var invalid []PreflightResult

	res, ok := injectAndCheck(src, filePath, ptrs)
	if !ok {
		for i := range mutants {
			single, singleOK := injectAndCheck(src, filePath, []*Mutant{&mutants[i]})
			if !singleOK {
				invalid = append(invalid, PreflightResult{
					MutantID:    mutants[i].ID,
					Status:      StatusCompileError,
					ErrorReason: "schemata produced code that cannot be formatted or parsed",
				})
				continue
			}
			if reason, notApplied := single.NotApplied[mutants[i].ID]; notApplied {
				invalid = append(invalid, PreflightResult{
					MutantID:    mutants[i].ID,
					Status:      StatusInvalid,
					ErrorReason: "not injectable: " + reason,
				})
				continue
			}
			valid = append(valid, mutants[i])
		}
		return valid, invalid
	}

	for i := range mutants {
		m := mutants[i]
		if reason, notApplied := res.NotApplied[m.ID]; notApplied || !res.Applied[m.ID] {
			if reason == "" {
				reason = "mutation was not injected"
			}
			invalid = append(invalid, PreflightResult{
				MutantID:    m.ID,
				Status:      StatusInvalid,
				ErrorReason: "not injectable: " + reason,
			})
			continue
		}
		if pm, ok := res.Positions[m.ID]; ok {
			m.TempLine, m.TempCol = pm.TempLine, pm.TempCol
		}
		valid = append(valid, m)
	}
	return valid, invalid
}

func injectAndCheck(src []byte, filePath string, mutants []*Mutant) (*ApplyResult, bool) {
	out, res, err := ApplySchemataToSource(src, filePath, mutants)
	if err != nil || res == nil {
		return res, false
	}
	if _, err := parser.ParseFile(token.NewFileSet(), filePath, out, 0); err != nil {
		return res, false
	}
	return res, true
}

// ── Level 3 ──────────────────────────────────────────────────────────────────

// level3TypeCheckPreflight groups mutants by source file and type-checks each
// mutant's schemata-transformed AST using go/types.
//
// Imports are resolved via golang.org/x/tools/go/packages, which performs
// module-aware loading of the full transitive dependency graph. There is no
// lenient stubbing of missing packages and therefore no need to heuristically
// classify "stub-driven" errors versus real ones.
//
// A baseline of type errors present in the unmodified file is still computed
// and subtracted so that pre-existing bugs in user code (which Gorgon can't
// fix and shouldn't blame on the mutation) don't false-positive every mutant
// in the file.
func level3TypeCheckPreflight(mutants []Mutant, buildTags []string, log *logger.Logger) ([]Mutant, []PreflightResult) {
	// Group by source file.
	groups := make(map[string][]Mutant)
	for i := range mutants {
		if mutants[i].Site.File != nil {
			key := mutants[i].Site.File.Name()
			groups[key] = append(groups[key], mutants[i])
		}
	}

	cache := newPkgImportCache(buildTags)

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
			v, inv := typeCheckFileGroup(filePath, fileMutants, cache, buildTags, log)
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

var activeMutantID = gorgonActiveMutantIDFromEnv()

func gorgonActiveMutantIDFromEnv() int {
	id, _ := strconv.Atoi(os.Getenv("GORGON_MUTANT_ID"))
	return id
}
`

type siblingFile struct {
	path string
	src  []byte
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

func loadSiblingFiles(pkgDir, excludePath string, buildTags []string) []siblingFile {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	bctx := build.Default
	bctx.BuildTags = append([]string(nil), buildTags...)
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
		if match, err := bctx.MatchFile(pkgDir, name); err == nil && !match {
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

// ── Real importer backed by golang.org/x/tools/go/packages ─────────────────

// pkgImportCache loads each package directory exactly once via packages.Load
// (with NeedDeps + NeedTypes) and exposes the resulting transitive type info
// as a types.Importer. Subsequent type-check calls in the same directory
// reuse the cached load.
//
// Loading is the expensive step (full module-aware import graph). Sharing it
// across every mutant in a file — and across files in the same directory —
// keeps preflight latency proportional to package count, not mutant count.
type pkgImportCache struct {
	mu        sync.Mutex
	loaded    map[string]*resolvedImporter
	buildTags []string
}

func newPkgImportCache(buildTags []string) *pkgImportCache {
	return &pkgImportCache{loaded: make(map[string]*resolvedImporter), buildTags: buildTags}
}

// importerFor returns a types.Importer that resolves imports for the package
// rooted at pkgDir. The first call for a given directory triggers a load;
// subsequent calls return the cached importer. If loading fails for any
// reason (no go.mod, network-disabled module mode, etc.), a fallback to
// importer.Default() is returned — not a stub, so missing imports surface
// as real errors rather than being silently absorbed.
func (c *pkgImportCache) importerFor(pkgDir string) types.Importer {
	c.mu.Lock()
	if imp, ok := c.loaded[pkgDir]; ok {
		c.mu.Unlock()
		return imp
	}
	c.mu.Unlock()

	deps := loadPackageDeps(pkgDir, c.buildTags)
	imp := &resolvedImporter{deps: deps, fallback: importer.Default()}

	c.mu.Lock()
	c.loaded[pkgDir] = imp
	c.mu.Unlock()
	return imp
}

// loadPackageDeps invokes packages.Load for the package at pkgDir with full
// type information and returns a flat import-path → *types.Package map of
// every package reachable in its transitive dependency graph.
func loadPackageDeps(pkgDir string, buildTags []string) map[string]*types.Package {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedDeps | packages.NeedImports |
			packages.NeedTypesInfo,
		Dir:   pkgDir,
		Tests: false,
	}
	if len(buildTags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(buildTags, ",")}
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil || len(pkgs) == 0 {
		return nil
	}
	deps := make(map[string]*types.Package)
	var collect func(p *packages.Package)
	collect = func(p *packages.Package) {
		if p == nil || p.Types == nil {
			return
		}
		if _, seen := deps[p.PkgPath]; seen {
			return
		}
		deps[p.PkgPath] = p.Types
		for _, dep := range p.Imports {
			collect(dep)
		}
	}
	for _, p := range pkgs {
		collect(p)
	}
	return deps
}

// resolvedImporter resolves package paths against the dependency graph
// brought in by packages.Load. Stdlib paths missing from the graph are
// resolved via importer.Default() as a last resort. There is no stubbing —
// an unresolved path returns an error and the type-checker reports it.
type resolvedImporter struct {
	deps     map[string]*types.Package
	fallback types.Importer
}

func (r *resolvedImporter) Import(path string) (*types.Package, error) {
	if p, ok := r.deps[path]; ok {
		return p, nil
	}
	if r.fallback != nil {
		return r.fallback.Import(path)
	}
	return nil, fmt.Errorf("unresolved import: %s", path)
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

// isObviouslyUnsafeMutation rejects a mutant before any AST surgery if its
// operator has explicitly declared the site shape unsafe via the
// SafetyConstrainedOperator contract. There is no name-substring matching
// here: an operator that wants to opt out of preflight implements the
// interface in its own package; one that doesn't is passed through to the
// type-check phase, which is the authoritative validator.
func isObviouslyUnsafeMutation(m *Mutant) bool {
	if m.Site.Node == nil || m.Site.File == nil {
		return true
	}
	if m.Operator == nil {
		return false
	}
	if sc, ok := m.Operator.(mutator.SafetyConstrainedOperator); ok {
		return sc.IsAlwaysInvalidFor(m.Site.ReturnType)
	}
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

func typeCheckFileGroup(filePath string, mutants []Mutant, cache *pkgImportCache, buildTags []string, log *logger.Logger) ([]Mutant, []PreflightResult) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		log.Debug("[PREFLIGHT L3] Cannot read %s — skipping type-check", filePath)
		return mutants, nil
	}

	pkgDir := filepath.Dir(filePath)
	siblings := loadSiblingFiles(pkgDir, filePath, buildTags)
	pkgName := parsePackageName(src, filePath)
	helper := fmt.Sprintf(schemataHelperSrc, pkgName)
	imp := cache.importerFor(pkgDir)

	// Baseline: type errors present in the file with no mutation applied.
	// With real (non-stub) imports these are normally empty; when present,
	// they indicate pre-existing user-code errors that we must subtract so
	// they don't false-positive every mutant in the file.
	baseline := computeBaselineErrors(filePath, src, siblings, helper, pkgDir, imp)
	if log != nil && len(baseline) > 0 {
		log.Debug("[PREFLIGHT L3] %s: %d pre-existing baseline error message(s) — subtracting from mutant checks",
			filepath.Base(filePath), len(baseline))
	}

	mutPtrs := make([]*Mutant, len(mutants))
	for i := range mutants {
		mutPtrs[i] = &mutants[i]
	}

	errs, panicked := runTypeCheck(filePath, src, mutPtrs, siblings, helper, pkgDir, imp, baseline)
	if panicked {
		log.Debug("[PREFLIGHT L3] %s: go/types panicked on group of %d mutants, bisecting",
			filepath.Base(filePath), len(mutants))
		return bisectMutants(filePath, src, mutants, siblings, helper, pkgDir, imp, baseline, log)
	}
	if len(errs) > 0 {
		log.Debug("[PREFLIGHT L3] %s: %d type errors found in combined check, bisecting %d mutants",
			filepath.Base(filePath), len(errs), len(mutants))
		return bisectMutants(filePath, src, mutants, siblings, helper, pkgDir, imp, baseline, log)
	}
	return mutants, nil
}

// computeBaselineErrors returns a multiset (message → count) of type errors
// produced by go/types when checking the unmodified package. With a real
// (non-stub) importer this is normally empty; we subtract these from
// mutant-introduced errors so that pre-existing user-code typos don't
// false-positive every mutant in the file.
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

	// If left half panicked or had errors, bisect further to find bad mutants
	if leftPanicked || len(leftErrs) > 0 {
		v, inv := bisectMutants(filePath, src, left, siblings, helper, pkgDir, imp, baseline, log)
		valid = append(valid, v...)
		invalid = append(invalid, inv...)
	} else {
		valid = append(valid, left...)
	}

	// If right half panicked or had errors, bisect further to find bad mutants
	if rightPanicked || len(rightErrs) > 0 {
		v, inv := bisectMutants(filePath, src, right, siblings, helper, pkgDir, imp, baseline, log)
		valid = append(valid, v...)
		invalid = append(invalid, inv...)
	} else {
		valid = append(valid, right...)
	}

	return valid, invalid
}
