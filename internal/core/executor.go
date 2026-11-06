package testing

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
)

type ProgressTracker struct {
	total       int
	lastPrinted int
	mu          sync.Mutex
	done        int
}

func NewProgressTracker(total int) *ProgressTracker {
	return &ProgressTracker{total: total}
}

func (p *ProgressTracker) Record() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	if p.total <= 0 {
		return
	}
	pct := (p.done * 100) / p.total
	for p.lastPrinted < pct && pct > 0 {
		p.lastPrinted += 2
		fmt.Fprintf(os.Stderr, "Mutating [%d/%d %d%%]\n", p.done, p.total, p.lastPrinted)
	}
}

func (p *ProgressTracker) Finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done < p.total && p.total > 0 {
		pct := (p.done * 100) / p.total
		fmt.Fprintf(os.Stderr, "Mutating [%d/%d %d%%]\n", p.done, p.total, pct)
	}
	fmt.Fprintln(os.Stderr)
}

type mutantResult struct {
	id           int
	status       string
	err          error
	killedBy     string
	killDuration time.Duration
	killOutput   string
	reason       string
}

type schemataSpan struct {
	start, end int
	ids        []int
}

func findSchemataSpans(tempFile string) []schemataSpan {
	src, err := os.ReadFile(tempFile)
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, tempFile, src, parser.SkipObjectResolution)
	if file == nil {
		return nil
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }

	var spans []schemataSpan
	guardID := func(e ast.Expr) (int, bool) {
		be, ok := e.(*ast.BinaryExpr)
		if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
			return 0, false
		}
		id, ok := be.X.(*ast.Ident)
		if !ok || id.Name != "activeMutantID" {
			return 0, false
		}
		lit, ok := be.Y.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return 0, false
		}
		v, err := strconv.Atoi(lit.Value)
		return v, err == nil
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt:
			if id, ok := guardID(x.Cond); ok {
				spans = append(spans, schemataSpan{start: line(x.Pos()), end: line(x.Body.End()), ids: []int{id}})
				return true
			}
			ast.Inspect(x.Cond, func(c ast.Node) bool {
				if e, ok := c.(ast.Expr); ok {
					if id, ok := guardID(e); ok {
						spans = append(spans, schemataSpan{start: line(x.Cond.Pos()), end: line(x.Cond.End()), ids: []int{id}})
					}
				}
				return true
			})
		case *ast.ForStmt:
			if x.Cond != nil {
				ast.Inspect(x.Cond, func(c ast.Node) bool {
					if e, ok := c.(ast.Expr); ok {
						if id, ok := guardID(e); ok {
							spans = append(spans, schemataSpan{start: line(x.Cond.Pos()), end: line(x.Cond.End()), ids: []int{id}})
						}
					}
					return true
				})
			}
		case *ast.FuncLit:
			var ids []int
			for _, st := range x.Body.List {
				if ifs, ok := st.(*ast.IfStmt); ok {
					if id, ok := guardID(ifs.Cond); ok {
						ids = append(ids, id)
					}
				}
			}
			if len(ids) > 0 {
				spans = append(spans, schemataSpan{start: line(x.Pos()), end: line(x.End()), ids: ids})
			}
		}
		return true
	})
	return spans
}

func attributeErrors(tempDir string, errs []CompilerError, tempToSrc map[string]string, fileMutants map[string][]*Mutant) map[int]string {
	bad := make(map[int]string)
	spanCache := make(map[string][]schemataSpan)
	for _, ce := range errs {
		tf := absInWorkspace(tempDir, ce.File)
		src, ok := tempToSrc[tf]
		if !ok {
			continue
		}
		spans, ok := spanCache[tf]
		if !ok {
			spans = findSchemataSpans(tf)
			spanCache[tf] = spans
		}
		live := make(map[int]bool)
		for _, m := range fileMutants[src] {
			if m.Status == "" {
				live[m.ID] = true
			}
		}
		best := -1
		for i, sp := range spans {
			if ce.Line < sp.start || ce.Line > sp.end {
				continue
			}
			anyLive := false
			for _, id := range sp.ids {
				if live[id] {
					anyLive = true
				}
			}
			if !anyLive {
				continue
			}
			if best < 0 {
				best = i
				continue
			}
			b := spans[best]
			if (sp.end-sp.start) < (b.end-b.start) ||
				((sp.end-sp.start) == (b.end-b.start) && len(sp.ids) < len(b.ids)) {
				best = i
			}
		}
		if best < 0 {
			continue
		}
		msg := fmt.Sprintf("%s:%d:%d: %s", filepath.Base(ce.File), ce.Line, ce.Col, ce.Message)
		for _, id := range spans[best].ids {
			if live[id] {
				if prev, ok := bad[id]; ok {
					bad[id] = prev + "\n" + msg
				} else {
					bad[id] = msg
				}
			}
		}
	}
	return bad
}

func mutantTimeout(baseline time.Duration) time.Duration {
	t := time.Duration(float64(baseline)*timeoutMultiplier) + timeoutGrace
	if t > maxMutantTimeout {
		t = maxMutantTimeout
	}
	return t
}

type testBinary struct {
	path   string
	dir    string
	filter string
	label  string
}

func (b testBinary) baseline(ctx context.Context) (res runResult, ok bool) {
	runCtx, cancel := context.WithTimeout(ctx, baselineTimeout+hardTimeoutMargin)
	defer cancel()
	start := time.Now()
	out, err := runTestBinary(runCtx, b.path, b.dir, envWithMutant(0), b.filter, baselineTimeout)
	res = classifyVerboseRun(out, err, runCtx.Err() == context.DeadlineExceeded, false)
	res.duration = time.Since(start)
	if res.status != StatusSurvived {
		if res.status == StatusKilled {
			if names := failingTests(out); len(names) > 0 {
				res.killedBy = strings.Join(names, ", ")
			}
		}
		res.killOutput = truncOutput(out)
	}
	return res, res.status == StatusSurvived
}

func (b testBinary) runMutant(ctx context.Context, id int, timeout time.Duration) mutantResult {
	runCtx, cancel := context.WithTimeout(ctx, timeout+hardTimeoutMargin)
	defer cancel()
	start := time.Now()
	out, err := runTestBinary(runCtx, b.path, b.dir, envWithMutant(id), b.filter, timeout)
	dur := time.Since(start)
	if ctx.Err() != nil {
		return mutantResult{id: id, status: StatusError, err: ctx.Err(), reason: "run cancelled"}
	}
	r := classifyVerboseRun(out, err, runCtx.Err() == context.DeadlineExceeded, true)
	killedBy := r.killedBy
	if b.label != "" && (r.status == StatusKilled || r.status == StatusTimeout) {
		killedBy = strings.TrimSpace(killedBy + " [" + b.label + "]")
	}
	res := mutantResult{
		id:           id,
		status:       r.status,
		killedBy:     killedBy,
		killDuration: dur,
		killOutput:   r.killOutput,
	}
	if r.status == StatusError {
		res.err = err
		res.reason = r.killOutput
	}
	return res
}

func (r *run) testsFilter(origDir string) string {
	if len(r.testsByPkg) == 0 {
		return ""
	}
	abs, err := filepath.Abs(origDir)
	if err != nil {
		abs = origDir
	}
	tests, ok := r.testsByPkg[abs]
	if !ok {
		tests = r.testsByPkg[""]
	}
	if len(tests) == 0 {
		return ""
	}
	quoted := make([]string, len(tests))
	for i, t := range tests {
		quoted[i] = regexpQuote(t)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func regexpQuote(s string) string {
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, c) {
			b.WriteRune('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (r *run) runUnitTests(ctx context.Context, skipKilled bool) {
	byPkg := make(map[string][]*Mutant)
	for _, m := range r.pending() {
		dir, err := r.ws.tempPkgDir(m.Site.File.Name())
		if err != nil {
			setStatus(m, StatusError, "not run: "+err.Error())
			continue
		}
		byPkg[dir] = append(byPkg[dir], m)
	}
	if skipKilled {
		for i := range r.all {
			m := &r.all[i]
			if m.Status == StatusSurvived || m.Status == StatusUntested {
				if dir, err := r.ws.tempPkgDir(m.Site.File.Name()); err == nil {
					byPkg[dir] = append(byPkg[dir], m)
				}
			}
		}
	}

	binDir, err := os.MkdirTemp("", "gorgon-test-bins-*")
	if err != nil {
		for _, ms := range byPkg {
			for _, m := range ms {
				setStatus(m, StatusError, "not run: "+err.Error())
			}
		}
		return
	}
	defer os.RemoveAll(binDir)

	sem := make(chan struct{}, r.concurrent)
	var wg sync.WaitGroup
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(r.concurrent)

	pkgDirs := make([]string, 0, len(byPkg))
	for d := range byPkg {
		pkgDirs = append(pkgDirs, d)
	}
	sort.Strings(pkgDirs)

	var resMu sync.Mutex
	record := func(m *Mutant, res mutantResult) {
		resMu.Lock()
		mergeOutcome(m, res)
		resMu.Unlock()
		r.prog.Record()
	}

	for i, pkgDir := range pkgDirs {
		i, pkgDir := i, pkgDir
		ms := byPkg[pkgDir]
		sort.Slice(ms, func(a, b int) bool { return ms[a].ID < ms[b].ID })
		g.Go(func() error {
			bin, ok := r.preparePackage(gctx, pkgDir, ms, filepath.Join(binDir, fmt.Sprintf("pkg%d.test", i)), record)
			if !ok {
				return nil
			}
			base, passed := bin.baseline(gctx)
			if !passed {
				if base.status == StatusUntested {
					for _, m := range ms {
						record(m, mutantResult{id: m.ID, status: StatusUntested, reason: "no test ran in this package (with the configured test filter)"})
					}
					return nil
				}
				detail := base.killedBy
				if detail == "" {
					detail = base.status
				}
				r.log.Warn("[UNIT] baseline failed for %s (%s): its %d mutant(s) are not evaluated", relTo(r.ws.TempDir, pkgDir), detail, len(ms))
				for _, m := range ms {
					record(m, mutantResult{
						id:         m.ID,
						status:     StatusError,
						killOutput: base.killOutput,
						reason:     "not run: the package's tests fail without any mutation (baseline: " + detail + ")",
					})
				}
				return nil
			}
			timeout := mutantTimeout(base.duration)
			for _, m := range ms {
				m := m
				if skipKilled && m.Status == StatusKilled {
					continue
				}
				sem <- struct{}{}
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					record(m, bin.runMutant(ctx, m.ID, timeout))
				}()
			}
			return nil
		})
	}
	_ = g.Wait()
	wg.Wait()
}

func (r *run) preparePackage(ctx context.Context, pkgDir string, ms []*Mutant, binPath string, record func(*Mutant, mutantResult)) (testBinary, bool) {
	rel := "./" + filepath.ToSlash(relTo(r.ws.TempDir, pkgDir))
	if rel == "./." {
		rel = "."
	}
	hasTests, err := packageHasGoTestFiles(ctx, r.ws.TempDir, rel, r.buildTags)
	if err == nil && !hasTests {
		for _, m := range ms {
			record(m, mutantResult{id: m.ID, status: StatusUntested, reason: "package has no test files"})
		}
		return testBinary{}, false
	}

	args := []string{"test", "-c", "-vet=off"}
	if len(r.buildTags) > 0 {
		args = append(args, "-tags", strings.Join(r.buildTags, ","))
	}
	args = append(args, "-o", binPath, rel)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = r.ws.TempDir
	cmd.Env = envWithMutant(0)
	out, berr := cmd.CombinedOutput()
	if berr != nil {
		for _, m := range ms {
			record(m, mutantResult{
				id:         m.ID,
				status:     StatusError,
				killOutput: firstLines(string(out), 10),
				reason:     "not run: building the package's test binary failed: " + firstLines(string(out), 3),
			})
		}
		return testBinary{}, false
	}
	if _, err := os.Stat(binPath); err != nil {
		for _, m := range ms {
			record(m, mutantResult{id: m.ID, status: StatusUntested, reason: "package has no test files"})
		}
		return testBinary{}, false
	}
	origDir := filepath.Dir(ms[0].Site.File.Name())
	return testBinary{path: binPath, dir: pkgDir, filter: r.testsFilter(origDir)}, true
}

func relTo(base, p string) string {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return rel
}

func (r *run) runExternalSuites(ctx context.Context, cfg config.ExternalSuitesConfig) {
	// Create temp dir for binaries (outside workspace to avoid conflicts)
	binDir, err := os.MkdirTemp("", "gorgon-external-bins-*")
	if err != nil {
		r.log.Warn("[EXTERNAL] %v", err)
		return
	}
	defer os.RemoveAll(binDir)

	for _, suite := range cfg.Suites {
		suite := r.suiteInWorkspace(suite)
		resolved, _ := resolveSuitePaths(ctx, r.ws.TempDir, suite, r.log)
		if len(resolved) == 0 {
			r.log.Warn("[EXTERNAL] No packages found for suite %q", suite.Name)
			continue
		}
		binaries, err := buildExternalSuiteBinariesIn(ctx, r.ws.TempDir, binDir, suite, resolved, r.log)
		if err != nil {
			r.log.Warn("[EXTERNAL] Build failed for suite %q: %v", suite.Name, err)
			continue
		}
		pkgs := make([]string, 0, len(binaries))
		for p := range binaries {
			pkgs = append(pkgs, p)
		}
		sort.Strings(pkgs)

		for _, relPkg := range pkgs {
			bin := testBinary{
				path:  binaries[relPkg],
				dir:   filepath.Join(r.ws.TempDir, filepath.FromSlash(strings.TrimPrefix(relPkg, "./"))),
				label: suite.Name,
			}
			targets := r.externalTargets()
			if len(targets) == 0 {
				return
			}
			base, ok := bin.baseline(ctx)
			if !ok {
				r.log.Warn("[EXTERNAL] suite %q package %s does not pass without mutations (%s); not used", suite.Name, relPkg, base.status)
				continue
			}
			timeout := mutantTimeout(base.duration)
			r.log.Info("[EXTERNAL] %s %s: running %d mutant(s)", suite.Name, relPkg, len(targets))

			var mu sync.Mutex
			var wg sync.WaitGroup
			sem := make(chan struct{}, r.concurrent)
			for _, m := range targets {
				m := m
				sem <- struct{}{}
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					res := bin.runMutant(ctx, m.ID, timeout)
					mu.Lock()
					mergeOutcome(m, res)
					mu.Unlock()
				}()
			}
			wg.Wait()
		}
	}
}

func (r *run) externalTargets() []*Mutant {
	var out []*Mutant
	for i := range r.all {
		m := &r.all[i]
		switch m.Status {
		case "", StatusSurvived, StatusUntested, StatusError:
			if m.Status == StatusError && !r.injected(m) {
				continue
			}
			out = append(out, m)
		}
	}
	return out
}

func (r *run) injected(m *Mutant) bool {
	for _, x := range r.fileMutants[m.Site.File.Name()] {
		if x == m {
			return true
		}
	}
	return false
}

func (r *run) suiteInWorkspace(s config.ExternalSuite) config.ExternalSuite {
	out := s
	out.Paths = make([]string, 0, len(s.Paths))
	for _, p := range s.Paths {
		if !filepath.IsAbs(p) {
			out.Paths = append(out.Paths, p)
			continue
		}
		suffix := ""
		base := p
		if strings.HasSuffix(base, "/...") {
			suffix = "/..."
			base = strings.TrimSuffix(base, "/...")
		}
		rel, err := filepath.Rel(r.ws.absModule, base)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			r.log.Warn("[EXTERNAL] suite %q path %s is outside the module; it cannot observe mutations and is skipped", s.Name, p)
			continue
		}
		out.Paths = append(out.Paths, "./"+filepath.ToSlash(rel)+suffix)
	}
	return out
}

func resolveSuitePaths(ctx context.Context, workspaceDir string, suite config.ExternalSuite, log *logger.Logger) ([]string, error) {
	var resolved []string
	seen := make(map[string]bool)
	for _, p := range suite.Paths {
		args := []string{"list", "-f", "{{.Dir}}"}
		if len(suite.Tags) > 0 {
			args = append(args, "-tags", strings.Join(suite.Tags, ","))
		}
		// `go list` treats bare paths like "tests/integration/..." as import
		// paths, not directory patterns. Prefix with "./" so it's resolved
		// relative to workspaceDir.
		listArg := p
		if !strings.HasPrefix(listArg, "./") && !strings.HasPrefix(listArg, "/") &&
			!strings.HasPrefix(listArg, "../") && listArg != "." {
			listArg = "./" + listArg
		}
		args = append(args, listArg)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workspaceDir
		log.Debug("[EXTERNAL] Running: go %v in %s", args, workspaceDir)
		out, err := cmd.CombinedOutput()
		log.Debug("[EXTERNAL] Output: %s, Error: %v", string(out), err)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			rel, err := filepath.Rel(workspaceDir, line)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			pkg := "./" + filepath.ToSlash(rel)
			if !seen[pkg] {
				seen[pkg] = true
				resolved = append(resolved, pkg)
			}
		}
	}
	return resolved, nil
}

func buildExternalSuiteBinariesIn(ctx context.Context, workspaceDir, binDir string, suite config.ExternalSuite, resolvedPaths []string, log *logger.Logger) (map[string]string, error) {
	binaries := make(map[string]string, len(resolvedPaths))
	for _, relPkg := range resolvedPaths {
		safeName := strings.NewReplacer("/", "_", ".", "_").Replace(suite.Name + "_" + relPkg)
		binPath := filepath.Join(binDir, safeName+".test")

		args := []string{"test", "-c", "-vet=off", "-o", binPath}
		if len(suite.Tags) > 0 {
			args = append(args, "-tags", strings.Join(suite.Tags, ","))
		}
		args = append(args, relPkg)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workspaceDir
		cmd.Env = envWithMutant(0)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Warn("[EXTERNAL] building %s for suite %q failed: %s", relPkg, suite.Name, firstLines(string(out), 3))
			continue
		}
		if _, err := os.Stat(binPath); err != nil {
			continue
		}
		binaries[relPkg] = binPath
	}
	return binaries, nil
}

func packageNameInDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err == nil {
		fset := token.NewFileSet()
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "gorgon_schemata.go" {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly)
			if err == nil && f.Name != nil {
				return f.Name.Name
			}
		}
	}
	return filepath.Base(dir)
}
