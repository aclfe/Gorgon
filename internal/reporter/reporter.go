package reporter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aclfe/gorgon/internal/baseline"
	"github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/subconfig"
)

type BaselineOptions struct {
	Save         bool
	NoRegression bool
	Tolerance    float64
	Dir          string
	File         string
	MultiOutputs []string // format:file pairs from config
}

const (
	percentageMultiplier = 100
	tabWidth             = 4
)

func Report(mutants []testing.Mutant, totalMutants int, threshold float64, resolver *subconfig.Resolver, debug bool, showKilled bool, showSurvived bool, outputFile string, debugFile string, format string, blOpts BaselineOptions, writeTextToStdout bool) error {
	// Count statuses
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

	// Calculate score
	score := 0.0
	effectiveTotal := killed + survived + untested
	if effectiveTotal > 0 {
		score = float64(killed) / float64(effectiveTotal) * percentageMultiplier
	}

	// Write debug file if requested
	if debugFile != "" {
		if err := writeDebugInfo(mutants, killed, survived, errors, untested, debugFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write debug file: %v\n", err)
		}
	}

	// Write format-specific reports
	if outputFile != "" || format == "textfile" {
		switch format {
		case "textfile":
			if err := writeTextReport(mutants, totalMutants, threshold, resolver, debug, showKilled, showSurvived, outputFile); err != nil {
				return fmt.Errorf("failed to write text report: %w", err)
			}
		case "html":
			if err := writeHTMLReport(mutants, totalMutants, threshold, resolver, outputFile); err != nil {
				return fmt.Errorf("failed to write HTML report: %w", err)
			}
		case "junit":
			if err := writeJUnitReport(mutants, outputFile); err != nil {
				return fmt.Errorf("failed to write JUnit report: %w", err)
			}
		case "sarif":
			if err := writeSARIFReport(mutants, outputFile); err != nil {
				return fmt.Errorf("failed to write SARIF report: %w", err)
			}
		case "json":
			if err := writeJSONReport(mutants, totalMutants, score, killed, survived, errors, untested, outputFile); err != nil {
				return fmt.Errorf("failed to write JSON report: %w", err)
			}
		}
	}

	// Always write textfile to stdout when using multi-outputs
	if writeTextToStdout {
		if err := writeTextReport(mutants, totalMutants, threshold, resolver, debug, showKilled, showSurvived, ""); err != nil {
			return fmt.Errorf("failed to write text report to stdout: %w", err)
		}
	}

	// Handle multiple outputs from config (format:file pairs)
	if len(mutants) > 0 && blOpts.MultiOutputs != nil {
		for _, spec := range blOpts.MultiOutputs {
			parts := strings.SplitN(spec, ":", 2)
			if len(parts) != 2 {
				continue
			}
			fmtType := strings.TrimSpace(parts[0])
			file := strings.TrimSpace(parts[1])
			if file == "" {
				continue
			}
			switch fmtType {
			case "textfile":
				if err := writeTextReport(mutants, totalMutants, threshold, resolver, debug, showKilled, showSurvived, file); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write text report to %s: %v\n", file, err)
				}
			case "html":
				if err := writeHTMLReport(mutants, totalMutants, threshold, resolver, file); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write HTML report to %s: %v\n", file, err)
				}
			case "junit":
				if err := writeJUnitReport(mutants, file); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write JUnit report to %s: %v\n", file, err)
				}
			case "sarif":
				if err := writeSARIFReport(mutants, file); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write SARIF report to %s: %v\n", file, err)
				}
			case "json":
				if err := writeJSONReport(mutants, totalMutants, score, killed, survived, errors, untested, file); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write JSON report to %s: %v\n", file, err)
				}
			}
		}
	}

	// Baseline / ratchet handling
	if blOpts.Save || blOpts.NoRegression {
		current := &baseline.Data{
			Score:    score,
			Killed:   killed,
			Survived: survived,
			Untested: untested,
			Total:    totalMutants,
		}

		if blOpts.Save {
			if err := baseline.Save(blOpts.Dir, blOpts.File, current); err != nil {
				return fmt.Errorf("failed to save baseline: %w", err)
			}
			path := blOpts.File
			if path == "" {
				path = baseline.DefaultFile
			}
			fmt.Fprintf(os.Stdout, "\nBaseline saved: %.2f%% → %s\n", score, path)
		}

		if blOpts.NoRegression {
			saved, err := baseline.Load(blOpts.Dir, blOpts.File)
			if err != nil {
				// No baseline yet — auto-save and continue (golangci-lint trick)
				if os.IsNotExist(err) {
					if saveErr := baseline.Save(blOpts.Dir, blOpts.File, current); saveErr != nil {
						return fmt.Errorf("failed to auto-save baseline: %w", saveErr)
					}
					path := blOpts.File
					if path == "" {
						path = baseline.DefaultFile
					}
					fmt.Fprintf(os.Stdout, "\nNo baseline found — saved current score %.2f%% as baseline: %s\n", score, path)
				} else {
					return fmt.Errorf("failed to load baseline: %w", err)
				}
			} else {
				if err := baseline.CheckRegression(current, saved, blOpts.Tolerance); err != nil {
					return err
				}
				fmt.Fprintf(os.Stdout, "\nBaseline check passed: %.2f%% ≥ %.2f%% (tolerance: %.2f%%)\n",
					score, saved.Score, blOpts.Tolerance)
			}
		}
	}

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
