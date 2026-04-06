// Package main provides the gorgon command-line tool.
package main

import (
	"os"

	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/runner"
	_ "github.com/aclfe/gorgon/pkg/mutator/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/logical_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_body_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_break_first"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/variable_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/zero_value_return"
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
