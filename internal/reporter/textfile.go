package reporter

import (
	"fmt"
	"io"
	"os"
	"sort"
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

		if stats.CompileError+stats.Error > 0 {
			fmt.Fprintf(out, "\nError Summary by Operator:\n")
			opErrors := make(map[string]int)
			opTotal := make(map[string]int)
			for _, mutant := range mutants {
				opTotal[mutant.Operator.Name()]++
				if mutant.Status == testing.StatusError {
					opErrors[mutant.Operator.Name()]++
				}
			}
			for op, errCount := range opErrors {
				total := opTotal[op]
				pct := float64(errCount) / float64(total) * 100
				fmt.Fprintf(out, "  %-35s %d/%d errors (%.1f%%)\n", op, errCount, total, pct)
			}

			uniqueErrors := extractUniqueCompilerErrors(mutants)
			if len(uniqueErrors) > 0 {
				fmt.Fprintf(out, "\nTop Compilation Error Types (showing up to 20 of %d unique error messages):\n", len(uniqueErrors))
				for i, errMsg := range uniqueErrors {
					if i >= 20 {
						fmt.Fprintf(out, "  ... and %d more unique error types\n", len(uniqueErrors)-20)
						break
					}
					fmt.Fprintf(out, "  • %s\n", errMsg)
				}
			}

			fmt.Fprintf(out, "\nPer-Mutant Compilation Errors:\n")
			shownCount := writePerMutantErrors(out, mutants, 200)
			if shownCount > 200 {
				fmt.Fprintf(out, "  ... and %d more unique error lines (total: %d)\n", shownCount-200, shownCount)
			} else if shownCount == 0 {
				fmt.Fprintln(out, "  (no detailed errors available)")
			}

			fmt.Fprintf(out, "\nError Count by Operator:\n")
			opErrorCount := make(map[string]int)
			for _, mutant := range mutants {
				if mutant.Status == testing.StatusError {
					opErrorCount[mutant.Operator.Name()]++
				}
			}
			for op, count := range opErrorCount {
				fmt.Fprintf(out, "  %s: %d errors\n", op, count)
			}
		}

		fmt.Fprintln(out, "\n=== End Debug Information ===")
	}

	errors := stats.CompileError + stats.Error
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "Mutation Score\tKilled\tSurvived\tErrors\tTimeout\tUntested\tInvalid\tTotal")
	fmt.Fprintf(writer, "%.2f%%\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", stats.Score, stats.Killed, stats.Survived, errors, stats.Timeout, stats.Untested, stats.Invalid, stats.Total)
	writer.Flush()

	if stats.Killed > 0 {
		fmt.Fprintln(out, "\nTop Killing Tests:")
		testKills := make(map[string]int)
		for _, mutant := range mutants {
			if mutant.Status == testing.StatusKilled && mutant.KilledBy != "" {
				testKills[mutant.KilledBy]++
			}
		}

		type testKill struct {
			name  string
			count int
		}
		sortedTests := make([]testKill, 0, len(testKills))
		for name, count := range testKills {
			sortedTests = append(sortedTests, testKill{name, count})
		}
		sort.Slice(sortedTests, func(i, j int) bool {
			return sortedTests[i].count > sortedTests[j].count
		})

		maxShow := 10
		if len(sortedTests) < maxShow {
			maxShow = len(sortedTests)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Fprintf(out, "  %-50s %d kills\n", sortedTests[i].name, sortedTests[i].count)
		}
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
