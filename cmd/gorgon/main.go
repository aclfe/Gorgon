// Package main provides the gorgon command-line tool.
package main

import (
	"os"

	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/runner"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/empty_body"
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

func main() {
	flags, err := cli.Parse(os.Args[1:])
	if err != nil {
		runner.ExitWithError(err)
	}

	if err := flags.ValidateChecks(); err != nil {
		runner.ExitWithError(err)
	}

	// If no targets provided and not using config, print usage
	if len(flags.Targets) == 0 && flags.ConfigFile == "" && flags.PkgPath == "." {
		cli.PrintUsage()
	}

	// Add pkgPath as target if no targets specified
	if len(flags.Targets) == 0 {
		flags.Targets = []string{flags.PkgPath}
	}

	// Load configuration (from YAML or flags)
	cfg, err := flags.LoadConfig()
	if err != nil {
		runner.ExitWithError(err)
	}

	// Determine config path for suppression syncing
	configPath := flags.ConfigFile

	// Run the core logic
	if err := runner.Run(flags, cfg, flags.Targets, configPath); err != nil {
		// Error already handled in runner
		os.Exit(1)
	}
}
