// Package main provides the gorgon command-line tool.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/runner"
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

func main() {
	args := os.Args[1:]

	flags, err := cli.Parse(args)
	if err != nil {
		runner.ExitWithError(err)
	}

	if err := flags.ValidateChecks(); err != nil {
		runner.ExitWithError(err)
	}

	if len(flags.Targets) == 0 && flags.ConfigFile == "" && flags.PkgPath == "." {
		cli.PrintUsage()
	}

	if len(flags.Targets) == 0 {
		flags.Targets = []string{flags.PkgPath}
	}

	cfg, err := flags.LoadConfig()
	if err != nil {
		runner.ExitWithError(err)
	}

	var cpuProfileFile *os.File
	if cfg.CPUProfile != "" && cfg.CPUProfile != "false" {
		path := cfg.CPUProfile
		if path == "true" {
			path = "gorgon.cpuprofile"
		}
		cpuProfileFile, err = os.Create(path)
		if err != nil {
			runner.ExitWithError(fmt.Errorf("failed to create CPU profile: %w", err))
		}
		if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
			runner.ExitWithError(fmt.Errorf("failed to start CPU profile: %w", err))
		}
	}

	if cfg.MemProfile != "" {
		dir := cfg.MemProfile
		if err := os.MkdirAll(dir, 0o755); err != nil {
			runner.ExitWithError(fmt.Errorf("failed to create mem profile dir: %w", err))
		}
		fmt.Fprintf(os.Stderr, "[MEM PROFILER] STARTING - Dir: %s, Interval: 500ms\n", dir)

		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			start := time.Now()
			for now := range ticker.C {
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				heapMB := ms.HeapInuse / (1 << 20)

				elapsed := now.Sub(start)
				sec := int(elapsed.Seconds())
				msec := elapsed.Milliseconds() % 1000

				filename := fmt.Sprintf("%d_%03d_%dMB.prof", sec, msec, heapMB)
				profPath := filepath.Join(dir, filename)

				f, err := os.Create(profPath)
				if err == nil {
					pprof.WriteHeapProfile(f)
					f.Close()
					fmt.Fprintf(os.Stderr, "[MEM] wrote %s (%dMB heap)\n", filename, heapMB)
				} else {
					fmt.Fprintf(os.Stderr, "[MEM] error writing %s: %v\n", filename, err)
				}
			}
		}()
	}

	configPath := flags.ConfigFile

	runErr := runner.Run(flags, cfg, flags.Targets, configPath)

	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		cpuProfileFile.Close()
	}

	if runErr != nil {
		os.Exit(1)
	}
}
