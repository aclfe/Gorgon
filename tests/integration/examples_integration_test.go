//go:build integration
// +build integration

package integration

import (
	"path/filepath"
	"strings"
	"testing"

	assignment_operator_example "github.com/aclfe/gorgon/examples/mutations/assignment_operator"
	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
)

// ============================================================================
// ASSIGNMENT OPERATOR EXAMPLE
// ============================================================================

// TestAssignmentOperator_ExampleFunctions verifies the example functions behave
// correctly under multiple inputs. Each case is designed to catch a specific
// mutation: += -> -=, *= -> /=, /= -> *=.
func TestAssignmentOperator_ExampleFunctions(t *testing.T) {
	t.Run("AddToCounter_adds_positive", func(t *testing.T) {
		counter := 0
		assignment_operator_example.AddToCounter(&counter, 5)
		if counter != 5 {
			t.Errorf("AddToCounter(0, 5) = %d, want 5", counter)
		}
	})

	t.Run("AddToCounter_adds_negative", func(t *testing.T) {
		counter := 10
		assignment_operator_example.AddToCounter(&counter, -3)
		if counter != 7 {
			t.Errorf("AddToCounter(10, -3) = %d, want 7", counter)
		}
	})

	t.Run("Double_positive", func(t *testing.T) {
		if got := assignment_operator_example.Double(6); got != 12 {
			t.Errorf("Double(6) = %d, want 12", got)
		}
	})

	t.Run("Double_zero", func(t *testing.T) {
		if got := assignment_operator_example.Double(0); got != 0 {
			t.Errorf("Double(0) = %d, want 0", got)
		}
	})

	t.Run("Halve_even", func(t *testing.T) {
		if got := assignment_operator_example.Halve(8); got != 4 {
			t.Errorf("Halve(8) = %d, want 4", got)
		}
	})

	t.Run("Halve_zero_guard", func(t *testing.T) {
		if got := assignment_operator_example.Halve(0); got != 0 {
			t.Errorf("Halve(0) = %d, want 0", got)
		}
	})

	t.Run("Triple_positive", func(t *testing.T) {
		if got := assignment_operator_example.Triple(3); got != 9 {
			t.Errorf("Triple(3) = %d, want 9", got)
		}
	})
}

// ============================================================================
// REPORTER
// ============================================================================

// TestReporter_CalculateScore verifies the score formula and edge cases.
func TestReporter_CalculateScore(t *testing.T) {
	cases := []struct {
		killed, survived, untested, timeout int
		want                                float64
	}{
		{killed: 0, survived: 0, untested: 0, timeout: 0, want: 0},
		{killed: 10, survived: 0, untested: 0, timeout: 0, want: 100},
		{killed: 0, survived: 10, untested: 0, timeout: 0, want: 0},
		{killed: 50, survived: 50, untested: 0, timeout: 0, want: 50},
		{killed: 3, survived: 1, untested: 0, timeout: 0, want: 75},
		{killed: 80, survived: 10, untested: 10, timeout: 0, want: 80},
		{killed: 40, survived: 30, untested: 20, timeout: 10, want: 40},
	}

	for _, c := range cases {
		got := reporter.CalculateScore(c.killed, c.survived, c.untested, c.timeout)
		if got != c.want {
			t.Errorf("CalculateScore(%d,%d,%d,%d) = %.2f, want %.2f",
				c.killed, c.survived, c.untested, c.timeout, got, c.want)
		}
	}
}

// TestReporter_StatsForFile verifies StatsForFile counts statuses correctly and
// excludes errors/invalid from the score denominator.
func TestReporter_StatsForFile(t *testing.T) {
	op, ok := mutator.Get("assignment_operator")
	if !ok {
		t.Fatal("assignment_operator not registered")
	}

	makeMutant := func(status, killedBy string) coretesting.Mutant {
		return coretesting.Mutant{
			Operator: op,
			Status:   status,
			KilledBy: killedBy,
		}
	}

	mutants := []coretesting.Mutant{
		makeMutant(coretesting.StatusKilled, "TestFoo"),
		makeMutant(coretesting.StatusKilled, "TestBar"),
		makeMutant(coretesting.StatusSurvived, ""),
		makeMutant(coretesting.StatusUntested, ""),
		makeMutant(coretesting.StatusError, "(compiler)"),
		makeMutant(coretesting.StatusInvalid, ""),
	}

	stats := reporter.StatsForFile(mutants)

	if stats.Killed != 2 {
		t.Errorf("Killed = %d, want 2", stats.Killed)
	}
	if stats.Survived != 1 {
		t.Errorf("Survived = %d, want 1", stats.Survived)
	}
	if stats.Untested != 1 {
		t.Errorf("Untested = %d, want 1", stats.Untested)
	}
	if stats.CompileErrors != 1 {
		t.Errorf("CompileErrors = %d, want 1", stats.CompileErrors)
	}
	if stats.Invalid != 1 {
		t.Errorf("Invalid = %d, want 1", stats.Invalid)
	}
	if stats.TotalErrors != 1 {
		t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
	}

	// Score = 2 / (2+1+1) * 100 = 50 (errors and invalid excluded from denom)
	want := 50.0
	if stats.Score != want {
		t.Errorf("Score = %.2f, want %.2f", stats.Score, want)
	}
}

// ============================================================================
// ENGINE
// ============================================================================

// TestEngine_NewEngineAndProjectRoot verifies engine creation and project root
// resolution, including absolute path normalization.
func TestEngine_NewEngineAndProjectRoot(t *testing.T) {
	eng := engine.NewEngine(false)
	if eng == nil {
		t.Fatal("NewEngine returned nil")
	}

	if eng.ProjectRoot() != "" {
		t.Errorf("ProjectRoot before Set = %q, want empty", eng.ProjectRoot())
	}

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	eng.SetProjectRoot(repoRoot)
	if got := eng.ProjectRoot(); got != repoRoot {
		t.Errorf("ProjectRoot = %q, want %q", got, repoRoot)
	}

	// Relative path must be resolved to absolute.
	eng2 := engine.NewEngine(false)
	eng2.SetProjectRoot("../..")
	if !filepath.IsAbs(eng2.ProjectRoot()) {
		t.Errorf("ProjectRoot %q is not absolute after SetProjectRoot with relative path", eng2.ProjectRoot())
	}
}

// TestEngine_TraverseAssignmentOperatorFile verifies that Traverse on the
// assignment_operator example file discovers at least the expected number of
// mutation sites and that each site has a non-nil File pointer.
func TestEngine_TraverseAssignmentOperatorFile(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	targetFile := filepath.Join(repoRoot, "examples/mutations/assignment_operator/assignment_operator.go")

	op, ok := mutator.Get("assignment_operator")
	if !ok {
		t.Fatal("assignment_operator not registered")
	}

	eng := engine.NewEngine(false)
	eng.SetOperators([]mutator.Operator{op})
	eng.SetProjectRoot(repoRoot)

	if err := eng.Traverse(targetFile, nil); err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no sites found — expected assignment_operator to match at least one node")
	}

	// assignment_operator.go has: +=, *=, =, /= — at least 4 sites expected.
	if len(sites) < 4 {
		t.Errorf("got %d sites, want at least 4", len(sites))
	}

	for i, s := range sites {
		if s.File == nil {
			t.Errorf("site[%d].File is nil", i)
			continue
		}
		if !strings.HasSuffix(s.File.Name(), "assignment_operator.go") {
			t.Errorf("site[%d].File.Name() = %q, want suffix assignment_operator.go", i, s.File.Name())
		}
		if s.Line <= 0 {
			t.Errorf("site[%d].Line = %d, want > 0", i, s.Line)
		}
	}
}
