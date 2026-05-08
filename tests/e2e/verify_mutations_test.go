//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/tools/go/ast/astutil"

	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// =============================================================================
// TestVerifyMutations_GroundTruth
//
// Behavioural ground-truth verification of Gorgon's per-mutant classification.
//
// IMPORTANT — DESIGN STANCE
// -------------------------
// This test is *not* a mirror of internal/core/executor.go's classifier. The
// whole point is to detect bugs in that classifier and the surrounding
// schemata pipeline. The ground-truth side therefore:
//
//   * Uses **only authoritative Go toolchain signals**: build success/failure
//     from a compile-only `go test -run=^$` invocation, and RUN/PASS/FAIL
//     markers + exit code from the actual `go test` invocation.
//   * Does **not** scan for substring patterns like "syntax error",
//     "undefined:", etc. — those are Gorgon's heuristics and could be
//     wrong; we use a clean compile-vs-run split instead.
//   * Applies the mutation **directly** to the source AST (no schemata
//     wrapping, no activeMutantID gate). What lands on disk is exactly the
//     program a developer would have written if they typed the mutated form.
//   * Compares strictly. There is no equivalence collapsing — if Gorgon
//     reports KILLED but the mutated code actually survives all tests, the
//     test fails and reports the discrepancy.
//
// Pipeline
// --------
// For each mutant Gorgon produced:
//
//   1. Snapshot the original source file in a workspace copy of the repo.
//   2. Re-parse a fresh AST of that file.
//   3. Locate the mutation site by (line, column, AST node type), find the
//      enclosing FuncDecl in the same fresh AST, then call
//      mutator.ApplyOperator and replace the node.
//   4. Pretty-print the result back to the workspace file.
//   5. Run the *ideal* classifier against the mutant's package.
//   6. Restore the file and continue.
//
// Pre-conditions verified before any mutation:
//   * Every package containing a sampled mutant must build AND pass its
//     tests with no mutation applied. A flaky baseline would yield
//     meaningless ground truth.
//
// Knobs:
//   GORGON_VERIFY_MAX          sample size per run (default 300, 0 = all)
//   GORGON_VERIFY_SEED         RNG seed for sampling (default 1)
//   GORGON_VERIFY_PKG_TIMEOUT  per-package go-test timeout, seconds (default 90)
// =============================================================================

type bucket string

const (
	bKilled       bucket = "KILLED"        // tests ran; at least one failed
	bSurvived     bucket = "SURVIVED"      // tests ran; all passed
	bCompileError bucket = "COMPILE_ERROR" // build failed; no tests ran
	bTimeout      bucket = "TIMEOUT"       // wall-clock timeout
	bUntested     bucket = "UNTESTED"      // package has no tests / no tests ran
	bRuntimeError bucket = "RUNTIME_ERROR" // built, but binary crashed before any test executed
	bUnknown      bucket = "UNKNOWN"       // ground-truth derivation could not run
)

func TestVerifyMutations_GroundTruth(t *testing.T) {
	if os.Getenv("GORGON_MUTANT_ID") != "" {
		t.Skip("skipping ground-truth verification during mutation run")
	}
	if testing.Short() {
		t.Skip("skipping ground-truth verification in -short mode")
	}

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	targetDir := filepath.Join(repoRoot, "internal/core")

	// 1. Drive Gorgon's full pipeline so we have its classification per mutant.
	mutants := runFullPipelineForVerification(t, repoRoot, targetDir)
	if len(mutants) == 0 {
		t.Fatalf("no mutants produced for %s", targetDir)
	}

	// Stable order before sampling so the seed gives reproducible picks.
	sortMutantsByID(mutants)

	sampleSize := envInt("GORGON_VERIFY_MAX", 300)
	seed := envInt64("GORGON_VERIFY_SEED", 1)
	if sampleSize > 0 && sampleSize < len(mutants) {
		t.Logf("sampling %d/%d mutants (seed=%d, override with GORGON_VERIFY_MAX=0)", sampleSize, len(mutants), seed)
		rng := rand.New(rand.NewSource(seed))
		rng.Shuffle(len(mutants), func(i, j int) { mutants[i], mutants[j] = mutants[j], mutants[i] })
		mutants = mutants[:sampleSize]
		// Re-sort the sample so per-package processing is contiguous.
		sortMutantsByFileLine(mutants)
	} else {
		t.Logf("verifying all %d mutants (this will take a while)", len(mutants))
	}

	workspace := setupVerifyWorkspace(t, repoRoot)
	t.Logf("workspace: %s", workspace)

	// 2. Verify the baseline test suite is green for every package we'll touch.
	pkgsToCheck := uniqueWorkspacePkgs(t, workspace, repoRoot, mutants)
	pkgTimeout := time.Duration(envInt("GORGON_VERIFY_PKG_TIMEOUT", 90)) * time.Second
	verifyBaseline(t, workspace, pkgsToCheck, pkgTimeout)

	// 3. Classify each mutant via direct AST mutation + ideal classifier.
	type result struct {
		mutant         coretesting.Mutant
		gorgon         bucket
		truth          bucket
		detail         string
		dump           string
		bucketMismatch bool   // gorgon bucket != truth bucket
		attrMismatch   bool   // both KILLED but Gorgon blamed a test that didn't fail
		truthFailing   []string // tests ground truth observed failing under this mutant
	}
	results := make([]result, 0, len(mutants))
	bucketMismatches := 0
	attrMismatches := 0
	dumpDir := filepath.Join(t.TempDir(), "mismatches")

	for i := range mutants {
		m := mutants[i]
		gorgonBucket := normalizeGorgonStatus(m)
		truthBucket, detail, fullOutput, mutatedSrc, originalSrc := groundTruthFor(t, workspace, repoRoot, m, pkgTimeout)

		r := result{
			mutant:       m,
			gorgon:       gorgonBucket,
			truth:        truthBucket,
			detail:       detail,
			truthFailing: parseFailingTests(fullOutput),
		}

		// (a) Bucket-level disagreement. The most common signal: Gorgon's
		// classification of the same mutation differs from what `go test`
		// observes when the mutation is applied directly.
		if gorgonBucket != truthBucket {
			r.bucketMismatch = true
			bucketMismatches++
		}

		// (b) Attribution-level check. If both sides agree the mutation is
		// killed, Gorgon's m.KilledBy must name one of the tests that
		// actually fail under direct mutation. If it doesn't, the schemata
		// pipeline credited the wrong test — the kill is "right answer for
		// the wrong reason", which is a real bug to surface.
		if !r.bucketMismatch && gorgonBucket == bKilled && truthBucket == bKilled {
			if !attributionMatches(m.KilledBy, r.truthFailing) {
				r.attrMismatch = true
				attrMismatches++
				r.detail = r.detail + fmt.Sprintf(" | ATTRIBUTION_MISMATCH: gorgon blamed %q, truth-failing=%v", m.KilledBy, r.truthFailing)
			}
		}

		if r.bucketMismatch || r.attrMismatch {
			r.dump = saveMismatchDump(t, dumpDir, m, gorgonBucket, truthBucket, detail, fullOutput, mutatedSrc, originalSrc, r.truthFailing)
		}
		results = append(results, r)
	}

	// 4. Per-mutant report.
	t.Logf("=== per-mutant verification (%d mutants) ===", len(results))
	for _, r := range results {
		marker := "OK  "
		switch {
		case r.bucketMismatch:
			marker = "FAIL"
		case r.attrMismatch:
			marker = "ATTR"
		}
		pos := positionOf(r.mutant)
		extra := r.detail
		if r.dump != "" {
			extra = extra + " | dump=" + r.dump
		}
		t.Logf("[%s] #%d op=%s %s gorgon=%s truth=%s killedBy=%q | %s",
			marker, r.mutant.ID, r.mutant.Operator.Name(), pos, r.gorgon, r.truth, r.mutant.KilledBy, extra)
	}

	// 5. Summarise mismatches by (gorgon → truth) pair so trends are visible.
	if bucketMismatches > 0 || attrMismatches > 0 {
		bucketCounts := make(map[string]int)
		for _, r := range results {
			if r.bucketMismatch {
				bucketCounts[fmt.Sprintf("%s→%s", r.gorgon, r.truth)]++
			}
		}
		if len(bucketCounts) > 0 {
			t.Logf("=== bucket mismatch summary (gorgon → ground truth) ===")
			keys := make([]string, 0, len(bucketCounts))
			for k := range bucketCounts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				t.Logf("  %-40s %d", k, bucketCounts[k])
			}
		}
		if attrMismatches > 0 {
			t.Logf("=== attribution mismatch summary ===")
			t.Logf("  %d KILLED mutants where Gorgon's blamed test was not among the actually-failing tests", attrMismatches)
		}
		t.Errorf("%d bucket disagreement(s), %d attribution mismatch(es) out of %d mutants (dumps in %s)",
			bucketMismatches, attrMismatches, len(results), dumpDir)
	}
}

// -----------------------------------------------------------------------------
// Pipeline driver — returns the full Mutant slice (not just MutantInfo).
// -----------------------------------------------------------------------------

func runFullPipelineForVerification(t *testing.T, repoRoot, targetDir string) []coretesting.Mutant {
	t.Helper()

	ops := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot)

	if err := eng.Traverse(targetDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", targetDir, err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites in %s", targetDir)
	}

	log := logger.New(false)
	resolver, _ := subconfig.Discover(repoRoot, "")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx, sites, ops, ops,
		repoRoot, repoRoot,
		nil, resolver,
		runtime.NumCPU(),
		nil, nil, nil,
		log, false, true,
		config.ExternalSuitesConfig{},
		&config.Config{},
	)
	if err != nil {
		t.Logf("Gorgon pipeline returned error (some mutants may still be valid): %v", err)
	}
	return mutants
}

// -----------------------------------------------------------------------------
// Ground-truth derivation for a single mutant.
// -----------------------------------------------------------------------------

func groundTruthFor(t *testing.T, workspace, repoRoot string, m coretesting.Mutant, pkgTimeout time.Duration) (b bucket, detail string, output string, mutatedSrc []byte, originalSrc []byte) {
	t.Helper()

	if m.Site.File == nil {
		return bUnknown, "mutant has no source file attached", "", nil, nil
	}

	originalPath := m.Site.File.Name()
	rel, err := filepath.Rel(repoRoot, originalPath)
	if err != nil {
		return bUnknown, fmt.Sprintf("rel: %v", err), "", nil, nil
	}
	workspacePath := filepath.Join(workspace, rel)

	originalSrc, err = os.ReadFile(workspacePath)
	if err != nil {
		return bUnknown, fmt.Sprintf("read workspace file: %v", err), "", nil, nil
	}
	defer func() {
		_ = os.WriteFile(workspacePath, originalSrc, 0o644)
	}()

	mutatedSrc, applyErr := buildMutatedSource(originalSrc, originalPath, m)
	if applyErr != nil {
		return bUnknown, "rebuild mutation: " + applyErr.Error(), "", nil, originalSrc
	}

	// Critical sanity check: if the mutated source is byte-identical to the
	// original, the operator was a silent no-op or our AST surgery failed
	// to land. Running `go test` on unchanged code would falsely "prove"
	// the mutation survives — which is exactly the kind of false-confidence
	// failure mode this test exists to catch.
	if bytes.Equal(originalSrc, mutatedSrc) {
		return bUnknown, "mutation produced byte-identical source — operator no-op or AST surgery failed", "", mutatedSrc, originalSrc
	}

	if err := os.WriteFile(workspacePath, mutatedSrc, 0o644); err != nil {
		return bUnknown, fmt.Sprintf("write mutated file: %v", err), "", mutatedSrc, originalSrc
	}

	pkgDir := filepath.Dir(workspacePath)
	pkgRel, _ := filepath.Rel(workspace, pkgDir)
	pkg := "./" + filepath.ToSlash(pkgRel)

	b, detail, output = classifyIdeal(workspace, pkg, pkgTimeout)
	return b, detail, output, mutatedSrc, originalSrc
}

// buildMutatedSource re-parses src into a fresh AST, locates the AST node
// matching the mutant's site (by line/column/node-type), applies the operator
// in place using a freshly-derived enclosing FuncDecl, and prints the result.
func buildMutatedSource(src []byte, filePath string, m coretesting.Mutant) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	wantLine := m.Site.Line
	wantCol := m.Site.Column
	wantTypeName := nodeTypeName(m.Site.Node)
	if wantTypeName == "" {
		return nil, fmt.Errorf("site has nil node")
	}

	var funcStack []*ast.FuncDecl
	applied := false

	astutil.Apply(f,
		func(c *astutil.Cursor) bool {
			if applied {
				return false
			}
			if fd, ok := c.Node().(*ast.FuncDecl); ok {
				funcStack = append(funcStack, fd)
			}
			return true
		},
		func(c *astutil.Cursor) bool {
			n := c.Node()
			defer func() {
				if fd, ok := n.(*ast.FuncDecl); ok && len(funcStack) > 0 && funcStack[len(funcStack)-1] == fd {
					funcStack = funcStack[:len(funcStack)-1]
				}
			}()
			if applied || n == nil || isNilNode(n) {
				return true
			}
			pos := fset.Position(n.Pos())
			if pos.Line != wantLine || pos.Column != wantCol {
				return true
			}
			if nodeTypeName(n) != wantTypeName {
				return true
			}
			var enclosing *ast.FuncDecl
			if len(funcStack) > 0 {
				enclosing = funcStack[len(funcStack)-1]
			}
			mutated := mutator.ApplyOperator(m.Operator, n, m.Site.ReturnType, f, enclosing)
			if mutated == nil || mutated == n {
				return true
			}
			mutatedNode, ok := mutated.(ast.Node)
			if !ok {
				return true
			}
			c.Replace(mutatedNode)
			applied = true
			return false
		},
	)

	if !applied {
		return nil, fmt.Errorf("could not locate site %s:%d:%d (%s) in re-parsed AST",
			filepath.Base(filePath), wantLine, wantCol, wantTypeName)
	}

	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&buf, fset, f); err != nil {
		return nil, fmt.Errorf("printer: %w", err)
	}
	return buf.Bytes(), nil
}

func nodeTypeName(n ast.Node) string {
	if n == nil {
		return ""
	}
	t := reflect.TypeOf(n)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func isNilNode(n ast.Node) bool {
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// -----------------------------------------------------------------------------
// Ideal classifier.
//
// Phase 1 — compile only:
//   `go test -run=^$ -count=1 pkg`
//   This compiles the package's test binary without running any tests.
//   Exit non-zero => COMPILE_ERROR. The check is authoritative because the
//   Go toolchain itself decides: no substring matching, no heuristics.
//
// Phase 2 — run:
//   `go test -count=1 -timeout=Ns pkg`
//   We then look at the output for go-test framework markers and the exit
//   code:
//     * "[no test files]"               => UNTESTED
//     * "no tests to run" + exit 0      => UNTESTED
//     * "=== RUN" or "--- PASS:" or "--- FAIL:" present => tests ran
//         - exit 0 => SURVIVED
//         - exit !=0 => KILLED
//     * tests-ran markers absent + exit !=0 => RUNTIME_ERROR
//         (binary built & started but crashed before any test executed —
//          e.g. a TestMain panic, an init() panic, or a non-test fatal.)
//     * tests-ran markers absent + exit 0 => UNTESTED
//   Context deadline exceeded => TIMEOUT.
// -----------------------------------------------------------------------------

var (
	reTestRUN     = regexp.MustCompile(`(?m)^=== RUN\b`)
	reTestPASS    = regexp.MustCompile(`(?m)^--- PASS:`)
	reTestFAIL    = regexp.MustCompile(`(?m)^--- FAIL:`)
	reNoTestFiles = regexp.MustCompile(`(?m)^\?\s+\S+\s+\[no test files\]`)
	reNoTestsToRun = regexp.MustCompile(`(?i)\bno tests to run\b`)
)

func classifyIdeal(workspaceDir, pkg string, timeout time.Duration) (bucket, string, string) {
	// ── Phase 1: build the test binary with no tests run. ─────────────────
	buildCtx, buildCancel := context.WithTimeout(context.Background(), timeout)
	defer buildCancel()

	buildCmd := exec.CommandContext(buildCtx, "go", "test", "-run=^$", "-count=1", pkg)
	buildCmd.Dir = workspaceDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildCtx.Err() == context.DeadlineExceeded {
		return bTimeout, "build phase timed out", string(buildOut)
	}
	if buildErr != nil {
		// `go test -run=^$` exits non-zero only when the package fails to
		// build. (Tests are explicitly skipped, so test failures cannot
		// produce a non-zero exit here.)
		return bCompileError, "build failed: " + truncate(string(buildOut), 240), string(buildOut)
	}

	// ── Phase 2: run the tests. ────────────────────────────────────────────
	runCtx, runCancel := context.WithTimeout(context.Background(), timeout+15*time.Second)
	defer runCancel()

	runCmd := exec.CommandContext(runCtx, "go", "test", "-count=1",
		fmt.Sprintf("-timeout=%ds", int(timeout.Seconds())), pkg)
	runCmd.Dir = workspaceDir
	out, runErr := runCmd.CombinedOutput()
	output := string(out)

	if runCtx.Err() == context.DeadlineExceeded {
		return bTimeout, "test run wall-clock timeout", output
	}

	if reNoTestFiles.MatchString(output) {
		return bUntested, "package has no test files", output
	}

	testsRan := reTestRUN.MatchString(output) || reTestPASS.MatchString(output) || reTestFAIL.MatchString(output)

	if !testsRan {
		// No "=== RUN" / "--- PASS:" / "--- FAIL:" — the test framework
		// never executed a single test function.
		if runErr != nil {
			return bRuntimeError, "binary crashed before any test ran: " + truncate(output, 240), output
		}
		// Exit 0 with no tests-ran markers → either filter-empty selection
		// or a package that has no Test* functions in any _test.go file.
		if reNoTestsToRun.MatchString(output) {
			return bUntested, "no tests to run", output
		}
		return bUntested, "no tests executed", output
	}

	if runErr != nil {
		return bKilled, "test failure: " + truncate(output, 240), output
	}
	return bSurvived, "go test passed", output
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " | ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// -----------------------------------------------------------------------------
// Map Gorgon's reported status into the same bucket vocabulary.
//
// This mapping describes what Gorgon *says* happened, not what we think
// should have happened. The whole point of the test is to compare these
// against the ideal ground truth.
// -----------------------------------------------------------------------------

func normalizeGorgonStatus(m coretesting.Mutant) bucket {
	if m.KilledBy == "(timeout)" {
		return bTimeout
	}
	if m.KilledBy == "(compiler)" {
		// Gorgon overloads this marker across both `killed` and `error`
		// statuses to mean "build failed for this mutant".
		return bCompileError
	}
	switch m.Status {
	case coretesting.StatusKilled:
		return bKilled
	case coretesting.StatusSurvived:
		return bSurvived
	case coretesting.StatusUntested:
		return bUntested
	case coretesting.StatusInvalid:
		return bCompileError
	case coretesting.StatusTimeout:
		return bTimeout
	case coretesting.StatusError:
		// Generic "error" with no compiler/timeout marker. Gorgon's executor
		// reaches this on:
		//   * isCompilationError(output) substring match → which we treat
		//     here as RUNTIME_ERROR rather than COMPILE_ERROR, because if it
		//     were a real compile error Gorgon would have set
		//     KilledBy="(compiler)" upstream. This is the bucket where
		//     classifier mistakes hide.
		//   * non-zero exit with empty output → runtime error.
		return bRuntimeError
	default:
		return bUnknown
	}
}

// -----------------------------------------------------------------------------
// Workspace setup & baseline verification.
// -----------------------------------------------------------------------------

func setupVerifyWorkspace(t *testing.T, repoRoot string) string {
	t.Helper()
	dst := t.TempDir()

	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"bin":          true,
		"profiles":     true,
	}

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoRoot, path)
		if rel == "." {
			return nil
		}
		topLevel := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if d.IsDir() && skipDirs[topLevel] {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
	if err != nil {
		t.Fatalf("workspace copy failed: %v", err)
	}
	return dst
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func uniqueWorkspacePkgs(t *testing.T, workspace, repoRoot string, mutants []coretesting.Mutant) []string {
	t.Helper()
	seen := make(map[string]bool)
	for i := range mutants {
		if mutants[i].Site.File == nil {
			continue
		}
		rel, err := filepath.Rel(repoRoot, mutants[i].Site.File.Name())
		if err != nil {
			continue
		}
		pkg := "./" + filepath.ToSlash(filepath.Dir(rel))
		seen[pkg] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// verifyBaseline sanity-checks that the unmodified workspace passes its own
// tests for every package we are about to mutate. If a package's tests fail
// or fail to build *without* any mutation, ground-truth verification for that
// package would be meaningless — every "killed" classification could just be
// a pre-existing test failure. Fail fast in that case.
func verifyBaseline(t *testing.T, workspace string, pkgs []string, timeout time.Duration) {
	t.Helper()
	t.Logf("=== baseline verification (%d package(s)) ===", len(pkgs))

	type pkgResult struct {
		pkg    string
		ok     bool
		bucket bucket
		detail string
	}
	results := make([]pkgResult, len(pkgs))

	var wg sync.WaitGroup
	limit := make(chan struct{}, runtime.NumCPU())
	for i, pkg := range pkgs {
		wg.Add(1)
		limit <- struct{}{}
		go func(i int, pkg string) {
			defer wg.Done()
			defer func() { <-limit }()
			b, detail, _ := classifyIdeal(workspace, pkg, timeout)
			ok := b == bSurvived || b == bUntested
			results[i] = pkgResult{pkg: pkg, ok: ok, bucket: b, detail: detail}
		}(i, pkg)
	}
	wg.Wait()

	bad := 0
	for _, r := range results {
		if r.ok {
			t.Logf("  baseline OK   %s -> %s", r.pkg, r.bucket)
		} else {
			bad++
			t.Errorf("  baseline FAIL %s -> %s (%s)", r.pkg, r.bucket, r.detail)
		}
	}
	if bad > 0 {
		t.Fatalf("%d package(s) fail their own tests with no mutation applied — ground truth would be meaningless. Fix the baseline first.", bad)
	}
}

// -----------------------------------------------------------------------------
// Failing-test parsing & kill attribution.
// -----------------------------------------------------------------------------

var reFailLine = regexp.MustCompile(`(?m)^--- FAIL:\s+(\S+)`)

// parseFailingTests extracts every test name reported as failing in the
// `go test` output (matches both top-level tests and subtests). Order is
// preserved; duplicates are collapsed.
func parseFailingTests(output string) []string {
	matches := reFailLine.FindAllStringSubmatch(output, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// attributionMatches reports whether Gorgon's claimed killer test
// (`m.KilledBy`) lines up with the set of tests that actually failed under
// direct mutation. Special markers like "(compiler)", "(timeout)",
// "runtime error" are not test names — for those we have no attribution
// claim to verify, so we accept.
//
// Subtest matching: we treat a parent name as matching any of its
// descendant subtests (and vice-versa). E.g. Gorgon may report "TestFoo"
// while ground truth shows "TestFoo/case_x" — that's still the same kill.
func attributionMatches(killedBy string, failing []string) bool {
	killedBy = strings.TrimSpace(killedBy)
	if killedBy == "" {
		return true // nothing to check
	}
	switch killedBy {
	case "(compiler)", "(timeout)", "runtime error",
		"(test output non-empty)", "(compilation/runtime error)":
		return true
	}
	for _, f := range failing {
		if f == killedBy ||
			strings.HasPrefix(f, killedBy+"/") ||
			strings.HasPrefix(killedBy, f+"/") {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Mismatch dumping for offline inspection.
// -----------------------------------------------------------------------------

func saveMismatchDump(t *testing.T, dir string, m coretesting.Mutant, gorgon, truth bucket, detail, output string, mutatedSrc, originalSrc []byte, truthFailing []string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	base := fmt.Sprintf("mutant-%04d-%s-%s-vs-%s", m.ID, m.Operator.Name(), gorgon, truth)
	dump := filepath.Join(dir, base+".txt")

	var b bytes.Buffer
	fmt.Fprintf(&b, "MUTANT #%d\n", m.ID)
	fmt.Fprintf(&b, "Operator   : %s\n", m.Operator.Name())
	fmt.Fprintf(&b, "Site       : %s:%d:%d\n", positionOf(m), m.Site.Line, m.Site.Column)
	fmt.Fprintf(&b, "Func       : %s\n", funcNameOf(m))
	fmt.Fprintf(&b, "ReturnType : %q\n", m.Site.ReturnType)
	fmt.Fprintf(&b, "NodeType   : %s\n", nodeTypeName(m.Site.Node))
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "Gorgon status : %s (raw=%q killedBy=%q)\n", gorgon, m.Status, m.KilledBy)
	fmt.Fprintf(&b, "Ground truth  : %s (%s)\n", truth, detail)
	if len(truthFailing) > 0 {
		fmt.Fprintf(&b, "Truth failing : %v\n", truthFailing)
	}
	if originalSrc != nil && mutatedSrc != nil {
		fmt.Fprintf(&b, "\n--- Mutation diff (original → mutated) ---\n%s\n", lineDiff(originalSrc, mutatedSrc))
	}
	if m.KillOutput != "" {
		fmt.Fprintf(&b, "\n--- Gorgon kill output ---\n%s\n", m.KillOutput)
	}
	if output != "" {
		fmt.Fprintf(&b, "\n--- Ground-truth go test output ---\n%s\n", output)
	}

	_ = os.WriteFile(dump, b.Bytes(), 0o644)

	if mutatedSrc != nil {
		_ = os.WriteFile(filepath.Join(dir, base+".mutated.go"), mutatedSrc, 0o644)
	}
	if originalSrc != nil {
		_ = os.WriteFile(filepath.Join(dir, base+".original.go"), originalSrc, 0o644)
	}
	return dump
}

// lineDiff produces a tiny human-readable line-by-line diff of two source
// snapshots — only the lines that differ are shown, with a few lines of
// context around each change. This is not a real unified-diff format, but
// it's enough for a reviewer to spot where the mutation landed and how
// invasive it was.
func lineDiff(a, b []byte) string {
	la := strings.Split(string(a), "\n")
	lb := strings.Split(string(b), "\n")

	// Find the line ranges that differ. Since mutations are usually a
	// single-line edit, we just walk both side-by-side.
	type change struct {
		line int
		old  string
		new  string
	}
	var changes []change
	max := len(la)
	if len(lb) > max {
		max = len(lb)
	}
	for i := 0; i < max; i++ {
		var oa, ob string
		if i < len(la) {
			oa = la[i]
		}
		if i < len(lb) {
			ob = lb[i]
		}
		if oa != ob {
			changes = append(changes, change{line: i + 1, old: oa, new: ob})
		}
	}
	if len(changes) == 0 {
		return "(no line-level diff — whitespace-only or reformatting change)"
	}

	var buf bytes.Buffer
	for _, c := range changes {
		fmt.Fprintf(&buf, "L%d:\n", c.line)
		fmt.Fprintf(&buf, "  - %s\n", c.old)
		fmt.Fprintf(&buf, "  + %s\n", c.new)
		if len(changes) > 20 {
			fmt.Fprintf(&buf, "  (... %d more changes ...)\n", len(changes)-20)
			break
		}
	}
	return buf.String()
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func positionOf(m coretesting.Mutant) string {
	if m.Site.File == nil {
		return "<unknown>"
	}
	return fmt.Sprintf("%s:%d:%d", filepath.Base(m.Site.File.Name()), m.Site.Line, m.Site.Column)
}

func funcNameOf(m coretesting.Mutant) string {
	if m.Site.EnclosingFunc != nil && m.Site.EnclosingFunc.Name != nil {
		return m.Site.EnclosingFunc.Name.Name
	}
	if m.Site.FunctionName != "" {
		return m.Site.FunctionName
	}
	return "<unknown>"
}

func sortMutantsByID(ms []coretesting.Mutant) {
	sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
}

func sortMutantsByFileLine(ms []coretesting.Mutant) {
	sort.SliceStable(ms, func(i, j int) bool {
		fi, fj := "", ""
		if ms[i].Site.File != nil {
			fi = ms[i].Site.File.Name()
		}
		if ms[j].Site.File != nil {
			fj = ms[j].Site.File.Name()
		}
		if fi != fj {
			return fi < fj
		}
		if ms[i].Site.Line != ms[j].Site.Line {
			return ms[i].Site.Line < ms[j].Site.Line
		}
		return ms[i].ID < ms[j].ID
	})
}
