package reporter

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/aclfe/gorgon/internal/core"
)

func writeTextReport(mutants []testing.Mutant, stats ReportStats, debug, showKilled, showSurvived bool, outputFile string) error {
	var outWriters []io.Writer
	if outputFile == "" {
		outWriters = append(outWriters, os.Stdout)
	} else {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		outWriters = append(outWriters, f)
	}

	out := io.MultiWriter(outWriters...)
	fileCache := make(map[string][]byte)

	if debug {
		fmt.Fprintln(out, "=== Debug Information ===")

		if stats.TotalErrors > 0 || stats.Untested > 0 {
			fmt.Fprint(out, FormatDebugErrors(mutants, stats))

			fmt.Fprintf(out, "\nPer-Mutant Compilation Errors:\n")
			shownCount := writePerMutantErrors(out, mutants, 200)
			if shownCount > 200 {
				fmt.Fprintf(out, "  ... and %d more unique error lines (total: %d)\n", shownCount-200, shownCount)
			} else if shownCount == 0 {
				fmt.Fprintln(out, "  (no detailed errors available)")
			}
		}

		fmt.Fprintln(out, "\n=== End Debug Information ===")
	}

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "Mutation Score\tKilled\tSurvived\tCompile Errors\tRuntime Errors\tTimeout\tUntested\tInvalid\tTotal")
	fmt.Fprintf(writer, "%.2f%%\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", stats.Score, stats.Killed, stats.Survived, stats.CompileErrors, stats.RuntimeErrors, stats.Timeout, stats.Untested, stats.Invalid, stats.Total)
	writer.Flush()

	if stats.Killed > 0 {
		fmt.Fprintf(out, "\n%s", FormatTopKillingTests(mutants, 10))
	}

	if stats.Killed > 0 && showKilled {
		fmt.Fprintln(out, "\nKilled Mutants:")
		for _, mutant := range mutants {
			if mutant.Status == testing.StatusKilled {
				col := getVisualColumn(fileCache, mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column)
				killedBy := mutant.KilledBy
				if killedBy == "" {
					killedBy = "(unknown)"
				}
				duration := ""
				if mutant.KillDuration > 0 {
					duration = mutant.KillDuration.Round(time.Millisecond).String()
				}
				fmt.Fprintf(out, "- #%d %s:%d:%d (%s) killed by %s (%s)\n",
					mutant.ID, mutant.Site.File.Name(), mutant.Site.Line, col,
					mutant.Operator.Name(), killedBy, duration)
			}
		}
	}

	if showSurvived {
		fmt.Fprintln(out, "\nSurvived Mutants:")
		hasSurvived := false
		for _, mutant := range mutants {
			if mutant.Status == testing.StatusSurvived {
				hasSurvived = true
				col := getVisualColumn(fileCache, mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column)
				fmt.Fprintf(out, "- %s in %s:%d:%d (Operator: %s)\n",
					mutant.Status, mutant.Site.File.Name(), mutant.Site.Line, col,
					mutant.Operator.Name())
			}
		}
		if !hasSurvived {
			fmt.Fprintln(out, "  (none)")
		}
	}

	return nil
}
