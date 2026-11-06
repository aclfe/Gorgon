package testing

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/mutator"

	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/concurrency"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/condition_negation"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_panic_recover"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/error_handling"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/function_call_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/logical_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_body_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_first"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/variable_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/zero_value_return"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func discover(t *testing.T, dir string) []Mutant {
	t.Helper()
	ops := mutator.ListAll()
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(repoRoot(t))
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatalf("traverse %s: %v", dir, err)
	}
	return GenerateMutants(eng.Sites(), ops, ops, repoRoot(t), nil, nil, logger.New(false))
}

func byFile(mutants []Mutant) map[string][]*Mutant {
	out := make(map[string][]*Mutant)
	for i := range mutants {
		f := mutants[i].Site.File.Name()
		out[f] = append(out[f], &mutants[i])
	}
	return out
}

func TestInjection_EveryMutantIsInjectedOrRejected(t *testing.T) {
	skipHeavy(t)
	root := repoRoot(t)
	dirs, _ := filepath.Glob(filepath.Join(root, "examples/mutations/*"))
	total, applied := 0, 0
	reasons := map[string]int{}
	for _, dir := range dirs {
		mutants := discover(t, dir)
		for file, ms := range byFile(mutants) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			out, res, err := ApplySchemataToSource(src, file, ms)
			if err != nil {
				t.Errorf("%s: %v", file, err)
				continue
			}
			if _, err := parser.ParseFile(token.NewFileSet(), file, out, 0); err != nil {
				t.Errorf("%s: generated code does not parse: %v", file, err)
			}
			for _, m := range ms {
				total++
				_, isApplied := res.Applied[m.ID]
				reason, notApplied := res.NotApplied[m.ID]
				switch {
				case isApplied && notApplied:
					t.Errorf("mutant %d (%s) both applied and not applied", m.ID, m.Operator.Name())
				case !isApplied && !notApplied:
					t.Errorf("mutant %d (%s) has no injection verdict", m.ID, m.Operator.Name())
				case isApplied:
					applied++
					if !strings.Contains(string(out), "activeMutantID == "+itoa(m.ID)) &&
						!strings.Contains(string(out), "activeMutantID != "+itoa(m.ID)) {
						t.Errorf("mutant %d (%s) reported applied but guard missing", m.ID, m.Operator.Name())
					}
				default:
					reasons[m.Operator.Name()+": "+reason]++
				}
			}
		}
	}
	keys := make([]string, 0, len(reasons))
	for k := range reasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("not injected: %s (x%d)", k, reasons[k])
	}
	t.Logf("%d/%d mutants injected", applied, total)
	if total == 0 {
		t.Fatal("no mutants generated from examples")
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
