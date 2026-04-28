//go:build integration
// +build integration

package integration

import (
	"context"
	"runtime"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/subconfig"
	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/pkg/config"
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

// runPipeline runs the full mutation testing pipeline on fixtureDir and returns
// the computed ReportStats. It does not write any output files or to stdout.
func runPipeline(t *testing.T, fixtureDir string) reporter.ReportStats {
	t.Helper()

	ops := mutator.ListAll()

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(fixtureDir)

	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse %s: %v", fixtureDir, err)
	}

	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites found in %s — check fixture", fixtureDir)
	}

	log := logger.New(false)
	resolver, _ := subconfig.Discover(fixtureDir, "")

	ctx := context.Background()
	mutants, err := coretesting.GenerateAndRunSchemata(
		ctx,
		sites,
		ops,
		ops,
		fixtureDir,
		fixtureDir,
		nil,
		resolver,
		runtime.NumCPU(),
		nil,
		nil,
		nil,
		log,
		false,
		true,
		config.ExternalSuitesConfig{},
		&config.Config{},
	)
	if err != nil {
		t.Logf("pipeline error (may be expected for some mutants): %v", err)
	}

	totalMutants := coretesting.GetTotalMutants()

	// format="" + outputFile="" means computeStats runs but nothing is written.
	stats, _ := reporter.Report(
		mutants,
		totalMutants,
		0,
		nil,
		false,
		false,
		false,
		"",
		"",
		"",
		reporter.BaselineOptions{},
	)
	return stats
}

