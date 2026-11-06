package testing

import (
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

func skipHeavy(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the go tool")
	}
	if os.Getenv("GORGON_MUTANT_ID") != "" {
		t.Skip("skipping nested pipeline during a mutation run")
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = "module example.com/m\n\ngo 1.22\n"
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func discoverWith(t *testing.T, dir string, ops []mutator.Operator, suppress []config.SuppressEntry) []Mutant {
	t.Helper()
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(dir)
	eng.SetSuppressEntries(suppress)
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatalf("traverse: %v", err)
	}
	return GenerateMutants(eng.Sites(), ops, ops, dir, nil, nil, logger.New(false))
}

func opsNamed(t *testing.T, names ...string) []mutator.Operator {
	t.Helper()
	var ops []mutator.Operator
	for _, n := range names {
		op, ok := mutator.Get(n)
		if !ok {
			t.Fatalf("operator %s not registered", n)
		}
		ops = append(ops, op)
	}
	return ops
}

func applyAll(t *testing.T, file string, ms []Mutant) (string, *ApplyResult) {
	t.Helper()
	ptrs := make([]*Mutant, 0, len(ms))
	for i := range ms {
		if ms[i].Site.File.Name() == file {
			ptrs = append(ptrs, &ms[i])
		}
	}
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	out, res, err := ApplySchemataToSource(src, file, ptrs)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return string(out), res
}

func typeCheck(t *testing.T, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "gen.go", src, 0)
	if err != nil {
		t.Fatalf("parse generated code: %v\n%s", err, src)
	}
	helper, _ := parser.ParseFile(fset, "helper.go", "package "+f.Name.Name+"\nvar activeMutantID int\n", 0)
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("p", fset, []*ast.File{f, helper}, nil); err != nil {
		t.Fatalf("generated code does not type-check: %v\n%s", err, src)
	}
}

func TestNestedMutationsAreAllInjected(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nfunc F(a, b, c int) int {\n\treturn a + b*c\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "arithmetic_flip"), nil)
	if len(ms) != 2 {
		t.Fatalf("want 2 mutants (outer + and inner *), got %d", len(ms))
	}
	out, res := applyAll(t, filepath.Join(dir, "a.go"), ms)
	for _, m := range ms {
		if !res.Applied[m.ID] {
			t.Errorf("mutant %d not injected: %s", m.ID, res.NotApplied[m.ID])
		}
	}
	typeCheck(t, out)
}

func TestChainedCallsAreDistinctSites(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\ntype B struct{}\n\nfunc (b B) X() B { return b }\nfunc (b B) Y() {}\n\nfunc F(b B) {\n\tb.X().Y()\n\tb.X().X().Y()\n}\n",
	})
	eng := engine.NewEngine(false)
	eng.SetOperators(mutator.ListAll())
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatal(err)
	}
	keys := make(map[engine.SiteKey]bool)
	for _, s := range eng.Sites() {
		k := s.Key()
		if keys[k] {
			t.Errorf("duplicate site key %+v", k)
		}
		keys[k] = true
	}
}

func TestEmptyBodyIsGuarded(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nvar n int\n\nfunc Inc() {\n\tn++\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "empty_body", "inc_dec_flip"), nil)
	out, res := applyAll(t, filepath.Join(dir, "a.go"), ms)
	if len(res.Applied) != 2 {
		t.Fatalf("want both mutants injected, got %v (not applied: %v)", res.Applied, res.NotApplied)
	}
	if !strings.Contains(out, "return") || !strings.Contains(out, "n++") {
		t.Fatalf("original body must stay, guarded by an early return:\n%s", out)
	}
	typeCheck(t, out)
}

func TestErrorReturnNilMultiValueCompiles(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nimport \"errors\"\n\nfunc F(x int) (int, error) {\n\tif x < 0 {\n\t\treturn 0, errors.New(\"neg\")\n\t}\n\treturn x, nil\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "error_return_nil"), nil)
	if len(ms) != 1 {
		t.Fatalf("want 1 error_return_nil mutant, got %d", len(ms))
	}
	out, res := applyAll(t, filepath.Join(dir, "a.go"), ms)
	if !res.Applied[ms[0].ID] {
		t.Fatalf("not injected: %s", res.NotApplied[ms[0].ID])
	}
	typeCheck(t, out)
}

func TestSwitchRemoveDefaultOnlySkipsDefault(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nfunc F(x int) string {\n\ts := \"\"\n\tswitch x {\n\tcase 1:\n\t\ts = \"one\"\n\tdefault:\n\t\ts = \"other\"\n\t}\n\treturn s\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "switch_remove_default"), nil)
	if len(ms) != 1 {
		t.Fatalf("want 1 mutant, got %d", len(ms))
	}
	out, res := applyAll(t, filepath.Join(dir, "a.go"), ms)
	if !res.Applied[ms[0].ID] {
		t.Fatalf("not injected: %s", res.NotApplied[ms[0].ID])
	}
	if strings.Contains(out, "return \"\"") {
		t.Fatalf("removing default must not inject an early return:\n%s", out)
	}
	if !strings.Contains(out, "} else {") {
		t.Fatalf("default body must run only when the mutant is inactive:\n%s", out)
	}
	typeCheck(t, out)
}

func TestIgnoreDirectivesRemoveOnlyThatOperator(t *testing.T) {
	src := "package m\n\nfunc F(a, b int) bool {\n\t//gorgon:ignore boundary_value\n\treturn a < b\n}\n"
	dir := writeModule(t, map[string]string{"a.go": src})
	ms := discoverWith(t, dir, opsNamed(t, "boundary_value", "condition_negation"), nil)
	for _, m := range ms {
		if m.Operator.Name() == "boundary_value" {
			t.Errorf("boundary_value should be ignored by the inline directive")
		}
	}
	if len(ms) != 1 || ms[0].Operator.Name() != "condition_negation" {
		t.Fatalf("want only condition_negation, got %d mutants", len(ms))
	}

	dir2 := writeModule(t, map[string]string{"a.go": strings.Replace(src, "\t//gorgon:ignore boundary_value\n", "", 1)})
	ms = discoverWith(t, dir2, opsNamed(t, "boundary_value", "condition_negation"),
		[]config.SuppressEntry{{Location: "a.go:4", Operators: []string{"condition_negation"}}})
	if len(ms) != 1 || ms[0].Operator.Name() != "boundary_value" {
		t.Fatalf("config suppress should remove condition_negation only, got %d mutants", len(ms))
	}
	ms = discoverWith(t, dir2, opsNamed(t, "boundary_value", "condition_negation"),
		[]config.SuppressEntry{{Location: "a.go:4"}})
	if len(ms) != 0 {
		t.Fatalf("suppress without operators should remove all, got %d", len(ms))
	}
}

func TestNoOpMutationsAreNotGenerated(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nfunc Z() int {\n\treturn 0\n}\n\nfunc H() int {\n\tx := 0x0\n\treturn x\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "zero_value_return_numeric", "constant_replacement"), nil)
	for _, m := range ms {
		if m.Operator.Name() == "zero_value_return_numeric" {
			t.Errorf("return 0 -> return 0 is not a mutation (line %d)", m.Site.Line)
		}
	}
	for _, m := range ms {
		mutated, changed := mutation(m.Operator, m.Site)
		if !changed {
			t.Errorf("mutant %d does not change the program", m.ID)
		}
		if lit, ok := mutated.(*ast.BasicLit); ok && lit.Value == "0" {
			t.Errorf("mutant %d replaces a zero literal with 0", m.ID)
		}
	}
}

func TestDuplicateReturnMutationsAreDropped(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nfunc F() int {\n\treturn 5\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "zero_value_return_numeric", "constant_replacement"), nil)
	if len(ms) != 1 {
		names := []string{}
		for _, m := range ms {
			names = append(names, m.Operator.Name())
		}
		t.Fatalf("return 5 -> return 0 must be generated once, got %v", names)
	}
}

func TestEquivalentConditionOperatorsDeduplicated(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nimport \"errors\"\n\nfunc F(err error) error {\n\tif err != nil {\n\t\treturn errors.New(\"x\")\n\t}\n\treturn nil\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "error_check_removal", "if_condition_false", "if_condition_true"), nil)
	seen := map[string]bool{}
	for _, m := range ms {
		seen[m.Operator.Name()] = true
	}
	if seen["if_condition_false"] {
		t.Errorf("if_condition_false duplicates error_check_removal and must be dropped")
	}
	if !seen["error_check_removal"] || !seen["if_condition_true"] {
		t.Errorf("want error_check_removal and if_condition_true, got %v", seen)
	}
}

func TestVariableReplacementSkipsDeclarations(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a.go": "package m\n\nfunc F(a, b int) int {\n\tvar c int = a\n\treturn c + b\n}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "variable_replacement"), nil)
	if len(ms) == 0 {
		t.Fatal("expected variable_replacement mutants on uses")
	}
	out, res := applyAll(t, filepath.Join(dir, "a.go"), ms)
	for _, m := range ms {
		if !res.Applied[m.ID] {
			t.Errorf("mutant %d at line %d not injected: %s", m.ID, m.Site.Line, res.NotApplied[m.ID])
		}
		if m.Site.Line == 3 {
			t.Errorf("parameter declaration at line 3 must not be a mutation site")
		}
	}
	typeCheck(t, out)
}

func TestClassifyVerboseRun(t *testing.T) {
	exitErr := func() error {
		err := exec.Command("sh", "-c", "exit 1").Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Skip("cannot produce an ExitError")
		}
		return err
	}()

	cases := []struct {
		name        string
		out         string
		err         error
		deadline    bool
		baselineRan bool
		want        string
	}{
		{"pass", "=== RUN   TestA\n--- PASS: TestA (0.00s)\nPASS\n", nil, false, true, StatusSurvived},
		{"fail", "=== RUN   TestA\n--- FAIL: TestA (0.00s)\nFAIL\n", exitErr, false, true, StatusKilled},
		{"timeout panic", "=== RUN   TestA\npanic: test timed out after 2s\n", exitErr, false, true, StatusTimeout},
		{"deadline", "=== RUN   TestA\n", exitErr, true, true, StatusTimeout},
		{"init crash", "panic: boom\n", exitErr, false, true, StatusKilled},
		{"no tests, baseline also none", "PASS\n", nil, false, false, StatusUntested},
		{"no tests under mutant", "PASS\n", nil, false, true, StatusSurvived},
		{"cannot start", "", errors.New("exec: not found"), false, true, StatusError},
		{"fail-looking output but exit 0", "=== RUN   TestA\n    --- FAIL: in a log line\nPASS\n", nil, false, true, StatusSurvived},
	}
	for _, c := range cases {
		got := classifyVerboseRun([]byte(c.out), c.err, c.deadline, c.baselineRan)
		if got.status != c.want {
			t.Errorf("%s: got %s want %s", c.name, got.status, c.want)
		}
	}
}

func TestMergeOutcomePrecedence(t *testing.T) {
	m := &Mutant{ID: 1}
	mergeOutcome(m, mutantResult{status: StatusSurvived})
	mergeOutcome(m, mutantResult{status: StatusError, reason: "suite infra"})
	if m.Status != StatusSurvived {
		t.Fatalf("an error must not override an executed outcome, got %s", m.Status)
	}
	mergeOutcome(m, mutantResult{status: StatusKilled, killedBy: "TestX"})
	if m.Status != StatusKilled || m.KilledBy != "TestX" {
		t.Fatalf("killed must win, got %s/%s", m.Status, m.KilledBy)
	}
	mergeOutcome(m, mutantResult{status: StatusTimeout})
	if m.Status != StatusKilled {
		t.Fatalf("killed is final among outcomes, got %s", m.Status)
	}

	inv := &Mutant{ID: 2, Status: StatusCompileError}
	mergeOutcome(inv, mutantResult{status: StatusKilled})
	if inv.Status != StatusCompileError {
		t.Fatalf("pre-run verdicts are final, got %s", inv.Status)
	}
}

func TestCacheKeyChangesWithTests(t *testing.T) {
	skipHeavy(t)
	dir := writeModule(t, map[string]string{
		"a.go":      "package m\n\nfunc F(a int) int { return a + 1 }\n",
		"a_test.go": "package m\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) {}\n",
	})
	ms := discoverWith(t, dir, opsNamed(t, "arithmetic_flip"), nil)
	if len(ms) != 1 {
		t.Fatalf("want 1 mutant, got %d", len(ms))
	}
	c := cache.New()
	key1 := newFingerprinter(dir, nil, "cfg").mutantKey(c, &ms[0])
	if err := os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package m\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { if F(1) != 2 { t.Fatal() } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	key2 := newFingerprinter(dir, nil, "cfg").mutantKey(c, &ms[0])
	if key1 == key2 {
		t.Fatal("changing a test file must change the cache key")
	}
	key3 := newFingerprinter(dir, nil, "other-cfg").mutantKey(c, &ms[0])
	if key3 == key2 {
		t.Fatal("changing the run configuration must change the cache key")
	}
}

func TestPipeline_EndToEnd(t *testing.T) {
	skipHeavy(t)
	dir := writeModule(t, map[string]string{
		"a/a.go":                 "package a\n\nvar registered bool\n\nfunc init() {\n\tregister()\n}\n\nfunc register() { registered = true }\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc IsPos(x int) bool {\n\treturn x > 0\n}\n",
		"a/a_test.go":            "package a\n\nimport \"testing\"\n\nfunc TestRegistered(t *testing.T) {\n\tif !registered {\n\t\tt.Fatal(\"not registered\")\n\t}\n}\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n",
		"b/b.go":                 "package b\n\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n",
		"b/b_test.go":            "package b\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) {\n\tt.Fatal(\"always fails\")\n}\n",
		"c/c.go":                 "package c\n\nfunc Mul(a, b int) int {\n\treturn a * b\n}\n",
		"d/d.go":                 "package d\n\nfunc Double(x int) int {\n\treturn x * 2\n}\n",
		"d/d_test.go":            "package d\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestDouble(t *testing.T) {\n\tif _, err := os.ReadFile(\"testdata/fixture.txt\"); err != nil {\n\t\tt.Fatal(err)\n\t}\n\tif Double(4) != 8 {\n\t\tt.Fatal(\"double\")\n\t}\n}\n",
		"d/testdata/fixture.txt": "data\n",
	})

	ops := opsNamed(t, "arithmetic_flip", "boundary_value", "function_call_removal")
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(dir)
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatal(err)
	}
	generated := GenerateMutants(eng.Sites(), ops, ops, dir, nil, nil, logger.New(false))

	results, err := GenerateAndRunSchemata(t.Context(), eng.Sites(), ops, ops, dir, dir, nil, nil, 4, nil,
		nil, nil, logger.New(false), false, true, config.ExternalSuitesConfig{}, &config.Config{})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if len(results) != len(generated) {
		t.Fatalf("every generated mutant must be reported exactly once: %d generated, %d reported", len(generated), len(results))
	}
	seen := map[int]bool{}
	for _, m := range results {
		if seen[m.ID] {
			t.Fatalf("mutant %d reported twice", m.ID)
		}
		seen[m.ID] = true
	}

	want := func(file, op, status string) {
		t.Helper()
		found := false
		for _, m := range results {
			if filepath.Base(m.Site.File.Name()) == file && m.Operator.Name() == op {
				found = true
				if m.Status != status {
					t.Errorf("%s %s: got %s (%s %s), want %s", file, op, m.Status, m.KilledBy, m.ErrorReason, status)
				}
			}
		}
		if !found {
			t.Errorf("no %s mutant in %s", op, file)
		}
	}
	want("a.go", "function_call_removal", StatusKilled)
	want("a.go", "arithmetic_flip", StatusKilled)
	want("a.go", "boundary_value", StatusSurvived)
	want("b.go", "arithmetic_flip", StatusError)
	want("c.go", "arithmetic_flip", StatusUntested)
	want("d.go", "arithmetic_flip", StatusKilled)
}
