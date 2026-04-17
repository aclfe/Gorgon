package reporter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/subconfig"
)

const (
	percentageMultiplier = 100
	tabWidth             = 4
)

func Report(mutants []testing.Mutant, totalMutants int, threshold float64, resolver *subconfig.Resolver, debug bool, showKilled bool, showSurvived bool, outputFile string, debugFile string) error {
	total := totalMutants
	killed := 0
	survived := 0
	errors := 0
	untested := 0
	unknown := 0

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
		default:
			unknown++
		}
	}

	fileCache := make(map[string][]byte)

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

	if debugFile != "" {
		if err := writeDebugInfo(mutants, killed, survived, errors, untested, debugFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write debug file: %v\n", err)
		}
	}

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
	if _, err := fmt.Fprintln(writer, "Mutation Score\tKilled\tSurvived\tErrors\tUntested\tTotal"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "%.2f%%\t%d\t%d\t%d\t%d\t%d\n", score, killed, survived, errors, untested, total); err != nil {
		return fmt.Errorf("failed to write stats: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

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

		shownCount := 0
		maxShow := 0
		for _, mutant := range mutants {
			if mutant.Status == "killed" && (maxShow == 0 || shownCount < maxShow) {
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
					mutant.ID,
					mutant.Site.File.Name(),
					mutant.Site.Line,
					col,
					mutant.Operator.Name(),
					killedBy,
					duration)
				shownCount++
			}
		}
		if maxShow > 0 && killed > maxShow {
			fmt.Fprintf(out, "  ... and %d more killed mutants\n", killed-maxShow)
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
					mutant.Status,
					mutant.Site.File.Name(),
					mutant.Site.Line,
					col,
					mutant.Operator.Name())
			}
		}
		if !hasSurvived {
			fmt.Fprintln(out, "  (none)")
		}
	}

	if threshold > 0 && effectiveTotal > 0 && score < threshold {
		// Check per-package thresholds if resolver is available
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

func checkPerPackageThresholds(mutants []testing.Mutant, rootThreshold float64, resolver *subconfig.Resolver, out io.Writer) error {
	// Group mutants by package directory
	type pkgStats struct {
		killed, survived, untested int
		sampleFile                 string
	}
	pkgs := make(map[string]*pkgStats)
	for _, m := range mutants {
		if m.Site.File == nil {
			continue
		}
		dir := filepath.Dir(m.Site.File.Name())
		if pkgs[dir] == nil {
			pkgs[dir] = &pkgStats{sampleFile: m.Site.File.Name()}
		}
		switch m.Status {
		case "killed":
			pkgs[dir].killed++
		case "survived":
			pkgs[dir].survived++
		case "untested":
			pkgs[dir].untested++
		}
	}

	var failures []string
	for dir, stats := range pkgs {
		effective := stats.killed + stats.survived + stats.untested
		if effective == 0 {
			continue
		}
		score := float64(stats.killed) / float64(effective) * percentageMultiplier
		threshold := resolver.EffectiveThreshold(stats.sampleFile, rootThreshold)
		if threshold > 0 && score < threshold {
			failures = append(failures,
				fmt.Sprintf(" %s: %.2f%% (threshold %.2f%%)", dir, score, threshold))
		}
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		fmt.Fprintln(out, "\nPackages below threshold:")
		for _, f := range failures {
			fmt.Fprintln(out, f)
		}
		return fmt.Errorf("%d package(s) below mutation score threshold", len(failures))
	}
	return nil
}

func extractUniqueCompilerErrors(mutants []testing.Mutant) []string {
	seen := make(map[string]bool)
	var unique []string
	prefix := "compilation failed: "
	for _, m := range mutants {
		if m.Status == "error" && m.Error != nil {
			msg := m.Error.Error()
			if idx := len(prefix); len(msg) > idx && msg[:idx] == prefix {
				msg = msg[idx:]
			}
			for _, errMsg := range testing.UniqueErrorLines(msg, "# ") {
				if !seen[errMsg] {
					seen[errMsg] = true
					unique = append(unique, errMsg)
				}
			}
		}
	}
	return unique
}

func extractCompilerOutput(errMsg string) string {

	prefixes := []string{
		"compilation failed (mutation detected):\n",
		"compilation failed in package:\n",
		"compilation failed (unparseable errors):\n",
		"compilation failed in package (see compiler output)\n",
		"compilation failed: ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(errMsg, prefix) {
			return errMsg[len(prefix):]
		}
	}
	return errMsg
}

func writePerMutantErrors(out io.Writer, mutants []testing.Mutant, maxLines int) int {
	seen := make(map[string]bool)
	shownCount := 0
	for _, mutant := range mutants {
		if mutant.Status != "error" || mutant.Error == nil {
			continue
		}
		errMsg := mutant.Error.Error()
		compilerOutput := extractCompilerOutput(errMsg)
		compilerErrors := testing.ParseCompilerErrors(compilerOutput)
		if len(compilerErrors) > 0 {
			for _, ce := range compilerErrors {
				line := fmt.Sprintf("%s:%d:%d: %s", filepath.Base(ce.File), ce.Line, ce.Col, ce.Message)
				if seen[line] {
					continue
				}
				seen[line] = true
				shownCount++
				if maxLines > 0 && shownCount > maxLines {
					return shownCount
				}
				fmt.Fprintf(out, "  (%s) %s\n", mutant.Operator.Name(), line)
			}
		} else {
			lines := strings.Split(compilerOutput, "\n")
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if l == "" || strings.HasPrefix(l, "# ") || strings.HasPrefix(l, "compilation failed") {
					continue
				}
				if seen[l] {
					continue
				}
				seen[l] = true
				shownCount++
				if maxLines > 0 && shownCount > maxLines {
					return shownCount
				}
				fmt.Fprintf(out, "  (%s) %s\n", mutant.Operator.Name(), l)
			}
		}
	}
	return shownCount
}

func calculateVisualColumn(content []byte, line, col int) int {
	start := 0
	currentLine := 1
	for i, b := range content {
		if currentLine == line {
			start = i
			break
		}
		if b == '\n' {
			currentLine++
		}
	}

	visualCol := 1
	for i := 0; i < col-1; i++ {
		if start+i >= len(content) {
			break
		}
		if content[start+i] == '\t' {
			visualCol += tabWidth - (visualCol-1)%tabWidth
		} else {
			visualCol++
		}
	}
	return visualCol
}

func getVisualColumn(fileCache map[string][]byte, fileName string, line, col int) int {
	if content, ok := fileCache[fileName]; ok {
		return calculateVisualColumn(content, line, col)
	}
	if content, err := os.ReadFile(fileName); err == nil {
		fileCache[fileName] = content
		return calculateVisualColumn(content, line, col)
	}
	return col
}

func writeDebugInfo(mutants []testing.Mutant, killed, survived, errors, untested int, debugFile string) error {
	f, err := os.OpenFile(debugFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open debug file: %w", err)
	}
	defer f.Close()

	out := f

	total := len(mutants)
	score := 0.0
	effectiveTotal := killed + survived
	if effectiveTotal > 0 {
		score = float64(killed) / float64(effectiveTotal) * percentageMultiplier
	}

	fmt.Fprintf(out, "Mutation Score: %.2f%%\n", score)
	fmt.Fprintf(out, "Killed: %d\n", killed)
	fmt.Fprintf(out, "Survived: %d\n", survived)
	fmt.Fprintf(out, "Errors: %d\n", errors)
	fmt.Fprintf(out, "Untested: %d\n", untested)
	fmt.Fprintf(out, "Total: %d\n\n", total)

	if errors > 0 || untested > 0 {
		fmt.Fprintf(out, "Error Summary by Operator:\n")
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
			fmt.Fprintf(out, "  %-35s %d/%d errors (%.1f%%)\n", op, errCount, total, pct)
		}

		if untested > 0 {
			fmt.Fprintf(out, "\nUntested by Operator (binary missing - package failed to compile):\n")
			opUntested := make(map[string]int)
			for _, mutant := range mutants {
				if mutant.Status == "untested" {
					opUntested[mutant.Operator.Name()]++
				}
			}
			for op, untestCount := range opUntested {
				total := opTotal[op]
				if total == 0 {
					total = 1
				}
				pct := float64(untestCount) / float64(total) * 100
				fmt.Fprintf(out, "  %-35s %d/%d untested (%.1f%%)\n", op, untestCount, total, pct)
			}
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

		fmt.Fprintf(out, "\nPer-Mutant Compilation Errors (unlimited):\n")
		shownCount := writePerMutantErrors(out, mutants, 0)
		if shownCount > 0 {
			fmt.Fprintf(out, "  (total: %d error lines)\n", shownCount)
		} else {
			fmt.Fprintln(out, "  (no detailed errors available)")
		}

		fmt.Fprintf(out, "\nError Count by Operator:\n")
		opErrorCount := make(map[string]int)
		for _, mutant := range mutants {
			if mutant.Status == "error" {
				opErrorCount[mutant.Operator.Name()]++
			}
		}
		for op, count := range opErrorCount {
			fmt.Fprintf(out, "  %s: %d errors\n", op, count)
		}
		fmt.Fprintln(out)
	}

	if killed > 0 {
		fmt.Fprintf(out, "Top Killing Tests:\n")
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
		fmt.Fprintln(out)
	}

	return nil
}
