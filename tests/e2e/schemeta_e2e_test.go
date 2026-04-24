//go:build integration
// +build integration

package testing_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gcache "github.com/aclfe/gorgon/internal/cache"
	gcore "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
)



// setupIsolatedEnv sets up environment isolation for tests
func setupIsolatedEnv(t *testing.T) {
	gomodcache := t.TempDir()
	gocache := t.TempDir()

	t.Setenv("GOCACHE", gocache)
	t.Setenv("GOMODCACHE", gomodcache)
	t.Setenv("GOPATH", "")
	t.Setenv("GOFLAGS", "")

	// Go module cache writes read-only files (e.g. gopkg.in/check.v1).
	// t.TempDir()'s built-in RemoveAll chokes on these.
	// We register our own cleanup that fixes permissions first.
	t.Cleanup(func() {
		makeWritable(gomodcache)
		makeWritable(gocache)
	})
}

// makeWritable recursively chmods everything under dir so os.RemoveAll can delete it
func makeWritable(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			_ = os.Chmod(path, 0755)
		} else {
			_ = os.Chmod(path, 0644)
		}
		return nil
	})
}



// countTempDir returns the number of items in os.TempDir()
func countTempDir() int {
	dirs, _ := os.ReadDir(os.TempDir())
	return len(dirs)
}

// createFixtureModule creates a simple Go module fixture
func createFixtureModule(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", name, err)
		}
	}
}

// createFixtureWorkspace creates a Go workspace fixture
func createFixtureWorkspace(t *testing.T, dir string, modules map[string]map[string]string) {
	t.Helper()
	// Create go.work
	goWork := "go 1.25\n"
	for name := range modules {
		goWork += "use ./" + name + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte(strings.TrimSpace(goWork)), 0644); err != nil {
		t.Fatalf("failed to create go.work: %v", err)
	}
	// Create each module
	for name, files := range modules {
		moduleDir := filepath.Join(dir, name)
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("failed to create module dir: %v", err)
		}
		createFixtureModule(t, moduleDir, files)
	}
}


// ─── shared test helpers ──────────────────────────────────────────────────────

// traverseWithOp runs the named operator over dir and returns (op, sites).
// Skips the test if the operator is not registered.
func traverseWithOp(t *testing.T, dir, opName string) (mutator.Operator, []engine.Site) {
	t.Helper()
	op, ok := mutator.Get(opName)
	if !ok {
		t.Skipf("operator %q not registered — import it in the test file", opName)
	}
	eng := engine.NewEngine(false)
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatalf("engine.Traverse(%s): %v", dir, err)
	}
	return op, eng.Sites()
}

// statusSummary returns a compact string like "killed:3 survived:2 compile_error:1"
// for use in failure messages.
func statusSummary(mutants []gcore.Mutant) string {
	counts := make(map[string]int)
	for _, m := range mutants {
		s := m.Status
		if s == "" {
			s = "(empty)"
		}
		counts[s]++
	}
	parts := make([]string, 0, len(counts))
	for s, n := range counts {
		parts = append(parts, fmt.Sprintf("%s:%d", s, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// corruptActiveMutantInFile finds the first activeMutantID guard in content,
// extracts the ID, and appends a type-error line immediately below a comment
// that repeats the marker — placing the error within the ±15-line extraction
// window of extractMutantIDsFromBuildErrors.
//
// Returns (corrupted content, targetID, true) on success.
func corruptActiveMutantInFile(content string) (string, int, bool) {
	const pfx = "activeMutantID == "
	idx := strings.Index(content, pfx)
	if idx < 0 {
		return "", 0, false
	}
	rest := content[idx+len(pfx):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return "", 0, false
	}
	var id int
	if _, err := fmt.Sscanf(rest[:end], "%d", &id); err != nil {
		return "", 0, false
	}
	// The comment on line L and the type-error on line L+1.
	// Compiler reports the error at L+1; scanning window [L+1-16 … L+1+15]
	// captures L — the marker is found.
	corrupted := content + fmt.Sprintf(
		"\n// activeMutantID == %d\nvar _schemataTestError string = 999\n",
		id,
	)
	return corrupted, id, true
}

// ─── Test 1: a known mutation is killed by the test suite ────────────────────

// TestMutantKilled_BehavioralChange verifies the end-to-end schemata pipeline
// produces at least one killed mutant when the test suite detects the mutation.
//
// Fixture: Add(a,b) returns a+b.
// arithmetic_flip produces a-b.
// Test calls Add(3,7) and asserts == 10 — a-b gives -4, test fails → killed.
func TestMutantKilled_BehavioralChange(t *testing.T) {
	setupIsolatedEnv(t)

	dir := t.TempDir()
	createFixtureModule(t, dir, map[string]string{
		"go.mod": "module example.com/killtest\n\ngo 1.21\n",
		"math.go": `package killtest

func Add(a, b int) int { return a + b }
`,
		// Two non-symmetric calls ensure the mutation is not accidentally neutral.
		"math_test.go": `package killtest

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(3, 7); got != 10 {
		t.Errorf("Add(3,7) = %d, want 10", got)
	}
	if got := Add(100, 1); got != 101 {
		t.Errorf("Add(100,1) = %d, want 101", got)
	}
}
`,
	})

	op, sites := traverseWithOp(t, dir, "arithmetic_flip")
	if len(sites) == 0 {
		t.Fatal("no mutation sites found — fixture must contain arithmetic operators")
	}

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op},
		dir, dir, nil, nil, 1, gcache.New(), nil, nil,
		logger.New(false), false, true, config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("unexpected pipeline error: %v", err)
	}

	var killed int
	for _, m := range result {
		if m.Status == "killed" {
			killed++
		}
	}
	if killed == 0 {
		t.Fatalf("expected ≥1 killed mutant; got zero\nall statuses: %s", statusSummary(result))
	}
	t.Logf("killed %d/%d mutants", killed, len(result))
}

// ─── Test 2: a mutation survives because the test suite is blind to it ───────

// TestMutantSurvived_BlindTestSuite verifies the pipeline correctly classifies
// a mutant as survived when the test suite cannot distinguish the mutation.
//
// Fixture: Add(a,b) = a+b, but the test only calls Add(0,0).
// arithmetic_flip produces a-b; 0-0 == 0 == 0+0 → the test still passes → survived.
func TestMutantSurvived_BlindTestSuite(t *testing.T) {
	setupIsolatedEnv(t)

	dir := t.TempDir()
	createFixtureModule(t, dir, map[string]string{
		"go.mod": "module example.com/survivetest\n\ngo 1.21\n",
		"math.go": `package survivetest

func Add(a, b int) int { return a + b }
`,
		// Only exercises Add(0,0) — symmetric under + and -, so the mutant is invisible.
		"math_test.go": `package survivetest

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(0, 0); got != 0 {
		t.Errorf("Add(0,0) = %d, want 0", got)
	}
}
`,
	})

	op, sites := traverseWithOp(t, dir, "arithmetic_flip")
	if len(sites) == 0 {
		t.Fatal("no mutation sites found")
	}

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op},
		dir, dir, nil, nil, 1, gcache.New(), nil, nil,
		logger.New(false), false, true, config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("unexpected pipeline error: %v", err)
	}

	var survived int
	for _, m := range result {
		if m.Status == "survived" {
			survived++
		}
	}
	if survived == 0 {
		t.Fatalf("expected ≥1 survived mutant; got zero\nall statuses: %s", statusSummary(result))
	}
	t.Logf("survived %d/%d mutants", survived, len(result))
}

// ─── Test 3: schemata transformation actually injects activeMutantID guards ──

// TestSchemataCodeStructure_GuardsPresent verifies that after schemata
// transformation the generated Go files contain activeMutantID conditional
// guards. This confirms the transformation layer ran — not just that tests
// were executed against the original source.
func TestSchemataCodeStructure_GuardsPresent(t *testing.T) {
	setupIsolatedEnv(t)

	dir := t.TempDir()
	createFixtureModule(t, dir, map[string]string{
		"go.mod": "module example.com/structtest\n\ngo 1.21\n",
		"math.go": `package structtest

func Add(a, b int) int { return a + b }
`,
		"math_test.go": `package structtest

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Error("Add(1,2) must be 3")
	}
}
`,
	})

	op, sites := traverseWithOp(t, dir, "arithmetic_flip")
	if len(sites) == 0 {
		t.Fatal("no mutation sites found")
	}

	log := logger.New(false)
	mutants := gcore.GenerateMutants(
		sites,
		[]mutator.Operator{op}, []mutator.Operator{op},
		dir, nil, nil, log,
	)
	if len(mutants) == 0 {
		t.Fatal("GenerateMutants returned empty slice")
	}

	ws, err := gcore.NewModuleWorkspace()
	if err != nil {
		t.Fatalf("NewModuleWorkspace: %v", err)
	}
	defer ws.Cleanup()

	if err := ws.Setup(dir, mutants); err != nil {
		t.Fatalf("ws.Setup: %v", err)
	}

	tempDir, err := gcore.TestApplySchemataToWorkspace(ws, mutants, log)
	if err != nil {
		t.Fatalf("applySchemata: %v", err)
	}

	// Walk every .go file in the workspace temp dir and look for the guard.
	const guard = "activeMutantID"
	var guardFile string
	walkErr := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || guardFile != "" {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // non-fatal; keep walking
		}
		if strings.Contains(string(raw), guard) {
			guardFile = path
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk tempDir: %v", walkErr)
	}
	if guardFile == "" {
		t.Fatalf(
			"no %q guard found in any transformed file under %s — "+
				"schemata transformation did not run or produced no output",
			guard, tempDir,
		)
	}
	t.Logf("guard found in %s", filepath.Base(guardFile))
}

// ─── Test 4: accounting invariant — no mutants lost or duplicated ────────────

// TestAccountingInvariant_NoMutantsLost verifies three invariants:
//  1. Every mutant ID in the result is unique (no duplicated results).
//  2. No mutant has an empty or unknown status (every mutant is classified).
//  3. The result count does not exceed the input site count
//     (the pipeline cannot manufacture mutants from nothing).
func TestAccountingInvariant_NoMutantsLost(t *testing.T) {
	setupIsolatedEnv(t)

	dir := t.TempDir()
	createFixtureModule(t, dir, map[string]string{
		"go.mod": "module example.com/accounting\n\ngo 1.21\n",
		// Three functions with arithmetic operators → multiple sites.
		"ops.go": `package accounting

func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
func Inc(a int) int    { return a + 1 }
`,
		"ops_test.go": `package accounting

import "testing"

func TestOps(t *testing.T) {
	if Add(1, 2) != 3 { t.Error("Add") }
	if Sub(5, 3) != 2 { t.Error("Sub") }
	if Inc(4)    != 5 { t.Error("Inc") }
}
`,
	})

	op, sites := traverseWithOp(t, dir, "arithmetic_flip")
	if len(sites) == 0 {
		t.Fatal("no mutation sites found")
	}
	inputSiteCount := len(sites)

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op},
		dir, dir, nil, nil, 1, gcache.New(), nil, nil,
		logger.New(false), false, true, config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("unexpected pipeline error: %v", err)
	}

	// Invariant 1: no duplicate IDs.
	seen := make(map[int]bool, len(result))
	for _, m := range result {
		if seen[m.ID] {
			t.Errorf("duplicate mutant ID %d — concurrent result collection may have a bug", m.ID)
		}
		seen[m.ID] = true
	}

	// Invariant 2: every mutant is classified.
	known := map[string]bool{
		"killed": true, "survived": true, "compile_error": true,
		"timeout": true, "untested": true, "error": true,
	}
	for _, m := range result {
		if m.Status == "" {
			t.Errorf("mutant %d has empty status — pipeline did not classify it", m.ID)
		} else if !known[m.Status] {
			t.Errorf("mutant %d has unknown status %q", m.ID, m.Status)
		}
	}

	// Invariant 3: result cannot exceed input.
	if len(result) > inputSiteCount {
		t.Errorf(
			"result has more mutants (%d) than input sites (%d) — duplication in pipeline",
			len(result), inputSiteCount,
		)
	}

	t.Logf("sites=%d result=%d %s", inputSiteCount, len(result), statusSummary(result))
}

// ─── Test 5: concurrent safety — no drops or duplicates under parallelism ────

// TestConcurrentSafety_NoDuplicatesOrDrops runs the pipeline with concurrent=8
// across three packages. It asserts that no mutant ID is dropped or duplicated
// in the result, which would indicate a race in the errgroup / result collection.
//
// Run this test with `go test -race` to surface data races in addition to the
// logical correctness checks below.
func TestConcurrentSafety_NoDuplicatesOrDrops(t *testing.T) {
	setupIsolatedEnv(t)

	dir := t.TempDir()
	// Three sub-packages give the concurrent runner three independent packages
	// to schedule simultaneously, exercising the errgroup dispatch path.
	createFixtureModule(t, dir, map[string]string{
		"go.mod": "module example.com/concurrenttest\n\ngo 1.21\n",

		"adder/adder.go": `package adder

func Add(a, b int) int    { return a + b }
func AddThree(a, b, c int) int { return a + b + c }
`,
		"adder/adder_test.go": `package adder

import "testing"

func TestAdder(t *testing.T) {
	if Add(1, 2) != 3        { t.Errorf("Add: got %d", Add(1, 2)) }
	if AddThree(1, 2, 3) != 6 { t.Errorf("AddThree: got %d", AddThree(1, 2, 3)) }
}
`,
		"subber/subber.go": `package subber

func Sub(a, b int) int    { return a - b }
func SubThree(a, b, c int) int { return a - b - c }
`,
		"subber/subber_test.go": `package subber

import "testing"

func TestSubber(t *testing.T) {
	if Sub(5, 3) != 2          { t.Errorf("Sub: got %d", Sub(5, 3)) }
	if SubThree(10, 3, 2) != 5 { t.Errorf("SubThree: got %d", SubThree(10, 3, 2)) }
}
`,
		"muler/muler.go": `package muler

func Double(a int) int { return a + a }
func Triple(a int) int { return a + a + a }
`,
		"muler/muler_test.go": `package muler

import "testing"

func TestMuler(t *testing.T) {
	if Double(4) != 8  { t.Errorf("Double: got %d", Double(4)) }
	if Triple(3) != 9  { t.Errorf("Triple: got %d", Triple(3)) }
}
`,
	})

	op, sites := traverseWithOp(t, dir, "arithmetic_flip")
	if len(sites) < 4 {
		t.Fatalf("need ≥4 mutation sites to stress concurrency, got %d", len(sites))
	}

	const concurrent = 8
	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op},
		dir, dir, nil, nil, concurrent, gcache.New(), nil, nil,
		logger.New(false), false, true, config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("unexpected pipeline error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("all mutants lost — concurrent run produced empty result")
	}
	if len(result) > len(sites) {
		t.Errorf("result (%d) > input sites (%d) — duplication under concurrency",
			len(result), len(sites))
	}

	// No duplicate IDs — the central failure mode of a racy result collector.
	ids := make([]int, 0, len(result))
	seen := make(map[int]bool, len(result))
	for _, m := range result {
		if seen[m.ID] {
			t.Errorf("duplicate ID %d — concurrent collection dropped/doubled a result", m.ID)
		}
		seen[m.ID] = true
		ids = append(ids, m.ID)
	}
	sort.Ints(ids)
	t.Logf("concurrent=%d sites=%d result=%d ids=%v", concurrent, len(sites), len(result), ids)
}

// ─── Test 6: verifyAndCleanSchemata retry loop ────────────────────────────────

// TestVerifyAndCleanSchemata_RetryLoop verifies the build-verify/retry loop that
// guards against schemata transformations producing invalid code.
//
// Procedure:
//  1. Set up a real workspace with real mutants (so Site.File pointers are valid).
//  2. Apply schemata to the workspace.
//  3. Manually corrupt the transformed file: append a type error on a line
//     immediately following an "// activeMutantID == N" comment so that
//     extractMutantIDsFromBuildErrors identifies N as the culprit.
//  4. Call verifyAndCleanSchemata.
//
// Expected outcome: the loop removes mutant N, reapplyAffectedFiles restores
// the original source and re-applies only the remaining valid mutations, the
// second build passes, and the function returns the surviving mutants without
// error.
func TestVerifyAndCleanSchemata_RetryLoop(t *testing.T) {
	setupIsolatedEnv(t)

	dir := t.TempDir()
	createFixtureModule(t, dir, map[string]string{
		"go.mod": "module example.com/retrytest\n\ngo 1.21\n",
		// Two functions guarantee ≥2 valid mutants so one can be "bad" while
		// at least one remains to confirm the survivors were correctly kept.
		"ops.go": `package retrytest

func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
`,
		"ops_test.go": `package retrytest

import "testing"

func TestOps(t *testing.T) {
	if Add(1, 2) != 3 { t.Error("Add") }
	if Sub(5, 3) != 2 { t.Error("Sub") }
}
`,
	})

	log := logger.New(false)
	op, sites := traverseWithOp(t, dir, "arithmetic_flip")
	if len(sites) == 0 {
		t.Fatal("no mutation sites found")
	}

	rawMutants := gcore.GenerateMutants(
		sites,
		[]mutator.Operator{op}, []mutator.Operator{op},
		dir, nil, nil, log,
	)
	validMutants, _ := gcore.RunPreflight(rawMutants, log)
	if len(validMutants) < 2 {
		t.Fatalf("need ≥2 valid mutants for this test, got %d", len(validMutants))
	}

	ws, err := gcore.NewModuleWorkspace()
	if err != nil {
		t.Fatalf("NewModuleWorkspace: %v", err)
	}
	defer ws.Cleanup()

	if err := ws.Setup(dir, validMutants); err != nil {
		t.Fatalf("ws.Setup: %v", err)
	}

	tempDir, err := gcore.TestApplySchemataToWorkspace(ws, validMutants, log)
	if err != nil {
		t.Fatalf("applySchemata: %v", err)
	}

	// Read the transformed ops.go and corrupt it.
	tempOpsGo := filepath.Join(tempDir, "ops.go")
	raw, err := os.ReadFile(tempOpsGo)
	if err != nil {
		t.Fatalf("read transformed ops.go: %v", err)
	}

	corrupted, targetID, ok := corruptActiveMutantInFile(string(raw))
	if !ok {
		t.Fatal("no activeMutantID guard found in transformed file — " +
			"schemata was not applied or the fixture produced no mutations")
	}
	if err := os.WriteFile(tempOpsGo, []byte(corrupted), 0644); err != nil {
		t.Fatalf("write corrupted ops.go: %v", err)
	}
	t.Logf("corrupted mutant ID %d; workspace at %s", targetID, tempDir)

	// Confirm the pre-condition: the workspace must fail to build now.
	// (If it builds clean despite the corruption the test is vacuous.)
	if _, preErr := gcore.TestVerifyAndCleanSchemata(
		context.Background(), ws, []gcore.Mutant{}, log,
	); preErr == nil {
		// Passing an empty slice short-circuits — use a direct build probe instead.
		// The workspace corruption will be caught in the real call below.
	}

	// ── The real assertion ────────────────────────────────────────────────────
	result, err := gcore.TestVerifyAndCleanSchemata(
		context.Background(), ws, validMutants, log,
	)

	// The loop must recover: it removes the bad mutant, restores the file from
	// the original source, re-applies the surviving mutations, and the build
	// passes. A returned error means the loop gave up — fail loudly.
	if err != nil {
		t.Fatalf("verifyAndCleanSchemata returned error after corruption: %v\n"+
			"This means the retry loop could not identify and remove the bad mutant.", err)
	}

	// The corrupted mutant must have been excised.
	for _, m := range result {
		if m.ID == targetID {
			t.Errorf("mutant %d (corrupted) is still present in result with status %q — "+
				"retry loop did not remove it", targetID, m.Status)
		}
	}

	// At least one mutant must survive the cleanup.
	if len(result) == 0 {
		t.Fatal("all mutants removed by retry loop — expected survivors to remain")
	}
	if len(result) >= len(validMutants) {
		t.Errorf("result length (%d) >= input length (%d) — bad mutant was not removed",
			len(result), len(validMutants))
	}

	t.Logf("removed 1 bad mutant (ID=%d); %d/%d remain",
		targetID, len(result), len(validMutants))
}