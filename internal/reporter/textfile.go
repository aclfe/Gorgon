package reporter

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/subconfig"
)

func writeTextReport(mutants []testing.Mutant, totalMutants int, threshold float64, resolver *subconfig.Resolver, debug, showKilled, showSurvived bool, outputFile string) error {
	killed := 0
	survived := 0
	errors := 0
	untested := 0

	for _, mutant := range mutants {
		switch mutant.Status {
		case "killed":
			killed++
		case "survived":
			survived++
		case "error":
			errors++
		case "untested":
			untested++
		}
	}

	var outWriters []io.Writer
	outWriters = append(outWriters, os.Stdout)

	var outFile *os.File
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		outFile = f
		outWriters = append(outWriters, f)
	}

	out := io.MultiWriter(outWriters...)
	fileCache := make(map[string][]byte)

	if debug {
		fmt.Fprintln(os.Stdout, "=== Debug Information ===")

		if errors > 0 {
			fmt.Fprintf(os.Stdout, "\nError Summary by Operator:\n")
			opErrors := make(map[string]int)
			opTotal := make(map[string]int)
			for _, mutant := range mutants {
				opTotal[mutant.Operator.Name()]++
				if mutant.Status == "error" {
					opErrors[mutant.Operator.Name()]++
				}
			}
			for op, errCount := range opErrors {
				total := opTotal[op]
				pct := float64(errCount) / float64(total) * 100
				fmt.Fprintf(os.Stdout, "  %-35s %d/%d errors (%.1f%%)\n", op, errCount, total, pct)
			}

			uniqueErrors := extractUniqueCompilerErrors(mutants)
			if len(uniqueErrors) > 0 {
				fmt.Fprintf(os.Stdout, "\nTop Compilation Error Types (showing up to 20 of %d unique error messages):\n", len(uniqueErrors))
				for i, errMsg := range uniqueErrors {
					if i >= 20 {
						fmt.Fprintf(os.Stdout, "  ... and %d more unique error types\n", len(uniqueErrors)-20)
						break
					}
					fmt.Fprintf(os.Stdout, "  • %s\n", errMsg)
				}
			}

			fmt.Fprintf(os.Stdout, "\nPer-Mutant Compilation Errors:\n")
			shownCount := writePerMutantErrors(os.Stdout, mutants, 200)
			if shownCount > 200 {
				fmt.Fprintf(os.Stdout, "  ... and %d more unique error lines (total: %d)\n", shownCount-200, shownCount)
			} else if shownCount == 0 {
				fmt.Fprintln(os.Stdout, "  (no detailed errors available)")
			}

			fmt.Fprintf(os.Stdout, "\nError Count by Operator:\n")
			opErrorCount := make(map[string]int)
			for _, mutant := range mutants {
				if mutant.Status == "error" {
					opErrorCount[mutant.Operator.Name()]++
				}
			}
			for op, count := range opErrorCount {
				fmt.Fprintf(os.Stdout, "  %s: %d errors\n", op, count)
			}
		}

		fmt.Fprintln(os.Stdout, "\n=== End Debug Information ===")
	}

	score := 0.0
	effectiveTotal := killed + survived + untested
	if effectiveTotal > 0 {
		score = float64(killed) / float64(effectiveTotal) * percentageMultiplier
	}

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "Mutation Score\tKilled\tSurvived\tErrors\tUntested\tTotal")
	fmt.Fprintf(writer, "%.2f%%\t%d\t%d\t%d\t%d\t%d\n", score, killed, survived, errors, untested, totalMutants)
	writer.Flush()

	if killed > 0 {
		fmt.Fprintln(out, "\nTop Killing Tests:")
		testKills := make(map[string]int)
		for _, mutant := range mutants {
			if mutant.Status == "killed" && mutant.KilledBy != "" {
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

	if killed > 0 && showKilled {
		fmt.Fprintln(out, "\nKilled Mutants:")
		for _, mutant := range mutants {
			if mutant.Status == "killed" {
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
			if mutant.Status == "survived" {
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

	if threshold > 0 && effectiveTotal > 0 && score < threshold {
		if resolver != nil && resolver.HasAnyOverrides() {
			if err := checkPerPackageThresholds(mutants, threshold, resolver, out); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("mutation score %.2f%% is below threshold %.2f%%", score, threshold)
		}
	}

	_ = outFile
	return nil
}
