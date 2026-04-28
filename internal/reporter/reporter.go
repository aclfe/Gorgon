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

// ReportStats holds all categorized mutant counts and the final score.
type ReportStats struct {
	Killed       int
	Survived     int
	Untested     int
	CompileError int
	Error        int
	Timeout      int
	Invalid      int
	Total        int
	Score        float64
}

const (
	percentageMultiplier = 100
	tabWidth             = 4
)

// computeStats counts mutant statuses and calculates the unified score.
// Score = Killed / (Killed + Survived + Untested + Timeout) * 100
// CompileError, Error, and Invalid are excluded from the score denominator.
func computeStats(mutants []testing.Mutant, totalMutants int) ReportStats {
	var s ReportStats
	s.Total = totalMutants
	for _, m := range mutants {
		switch m.Status {
		case testing.StatusKilled:
			s.Killed++
		case testing.StatusSurvived:
			s.Survived++
		case testing.StatusUntested:
			s.Untested++
		case testing.StatusTimeout:
			s.Timeout++
		case testing.StatusInvalid:
			s.Invalid++
		case testing.StatusError:
			if m.KilledBy == "(compiler)" {
				s.CompileError++
			} else {
				s.Error++
			}
		}
	}
	denom := s.Killed + s.Survived + s.Untested + s.Timeout
	if denom > 0 {
		s.Score = float64(s.Killed) / float64(denom) * percentageMultiplier
	}
	return s
}

func Report(mutants []testing.Mutant, totalMutants int, threshold float64, resolver *subconfig.Resolver, debug bool, showKilled bool, showSurvived bool, outputFile string, debugFile string, format string, blOpts BaselineOptions) (ReportStats, error) {
	stats := computeStats(mutants, totalMutants)

	// Baseline / ratchet handling - do this BEFORE threshold checks
	if blOpts.Save || blOpts.NoRegression {
		current := &baseline.Data{
			Score:    stats.Score,
			Killed:   stats.Killed,
			Survived: stats.Survived,
			Untested: stats.Untested,
			Total:    totalMutants,
		}

		if blOpts.Save {
			if err := baseline.Save(blOpts.Dir, blOpts.File, current); err != nil {
				return stats, fmt.Errorf("failed to save baseline: %w", err)
			}
			path := blOpts.File
			if path == "" {
				path = baseline.DefaultFile
			}
			fmt.Fprintf(os.Stdout, "\nBaseline saved: %.2f%% → %s\n", stats.Score, path)
		}

		if blOpts.NoRegression {
			saved, err := baseline.Load(blOpts.Dir, blOpts.File)
			if err != nil {
				if os.IsNotExist(err) {
					if saveErr := baseline.Save(blOpts.Dir, blOpts.File, current); saveErr != nil {
						return stats, fmt.Errorf("failed to auto-save baseline: %w", saveErr)
					}
					path := blOpts.File
					if path == "" {
						path = baseline.DefaultFile
					}
					fmt.Fprintf(os.Stdout, "\nNo baseline found — saved current score %.2f%% as baseline: %s\n", stats.Score, path)
				} else {
					return stats, fmt.Errorf("failed to load baseline: %w", err)
				}
			} else {
				if err := baseline.CheckRegression(current, saved, blOpts.Tolerance); err != nil {
					return stats, err
				}
				fmt.Fprintf(os.Stdout, "\nBaseline check passed: %.2f%% ≥ %.2f%% (tolerance: %.2f%%)\n",
					stats.Score, saved.Score, blOpts.Tolerance)
			}
		}
	}

	// Write debug file if requested
	if debugFile != "" {
		if err := writeDebugInfo(mutants, stats, debugFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write debug file: %v\n", err)
		}
	}

	// Write format-specific reports
	if outputFile != "" || format == "textfile" {
		switch format {
		case "textfile":
			if err := writeTextReport(mutants, stats, debug, showKilled, showSurvived, outputFile); err != nil {
				return stats, fmt.Errorf("failed to write text report: %w", err)
			}
		case "html":
			if err := writeHTMLReport(mutants, totalMutants, threshold, resolver, outputFile); err != nil {
				return stats, fmt.Errorf("failed to write HTML report: %w", err)
			}
		case "junit":
			if err := writeJUnitReport(mutants, outputFile); err != nil {
				return stats, fmt.Errorf("failed to write JUnit report: %w", err)
			}
		case "sarif":
			if err := writeSARIFReport(mutants, outputFile); err != nil {
				return stats, fmt.Errorf("failed to write SARIF report: %w", err)
			}
		case "json":
			if err := writeJSONReport(mutants, stats, outputFile); err != nil {
				return stats, fmt.Errorf("failed to write JSON report: %w", err)
			}
		}
	}

	// Always write text report to terminal exactly once.
	// If the legacy path already wrote to stdout (because outputFile was ""), skip it.
	if outputFile != "" || format != "textfile" {
		if err := writeTextReport(mutants, stats, debug, showKilled, showSurvived, ""); err != nil {
			return stats, fmt.Errorf("failed to write text report to terminal: %w", err)
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

			// Skip if this spec was already handled by the legacy path above
			if fmtType == format && file == outputFile {
				continue
			}

			switch fmtType {
			case "textfile":
				if err := writeTextReport(mutants, stats, debug, showKilled, showSurvived, file); err != nil {
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
				if err := writeJSONReport(mutants, stats, file); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write JSON report to %s: %v\n", file, err)
				}
			}
		}
	}

	// Centralized threshold check — applies regardless of output format
	if threshold > 0 {
		denom := stats.Killed + stats.Survived + stats.Untested + stats.Timeout
		if denom > 0 && stats.Score < threshold {
			if resolver != nil && resolver.HasAnyOverrides() {
				if err := checkPerPackageThresholds(mutants, threshold, resolver, os.Stdout); err != nil {
					return stats, err
				}
			} else {
				return stats, fmt.Errorf("mutation score %.2f%% is below threshold %.2f%%", stats.Score, threshold)
			}
		}
	}

	return stats, nil
}

func checkPerPackageThresholds(mutants []testing.Mutant, rootThreshold float64, resolver *subconfig.Resolver, out io.Writer) error {
	type pkgStats struct {
		killed, survived, untested, timeout int
		sampleFile                          string
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
		case testing.StatusKilled:
			pkgs[dir].killed++
		case testing.StatusSurvived:
			pkgs[dir].survived++
		case testing.StatusUntested:
			pkgs[dir].untested++
		case testing.StatusTimeout:
			pkgs[dir].timeout++
		}
	}

	var failures []string
	for dir, stats := range pkgs {
		denom := stats.killed + stats.survived + stats.untested + stats.timeout
		if denom == 0 {
			continue
		}
		score := float64(stats.killed) / float64(denom) * percentageMultiplier
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
		if m.Status == testing.StatusError && m.Error != nil {
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
		if mutant.Status != testing.StatusError || mutant.Error == nil {
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

func writeDebugInfo(mutants []testing.Mutant, stats ReportStats, debugFile string) error {
	f, err := os.OpenFile(debugFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open debug file: %w", err)
	}
	defer f.Close()

	out := f

	fmt.Fprintf(out, "Mutation Score: %.2f%%\n", stats.Score)
	fmt.Fprintf(out, "Killed: %d\n", stats.Killed)
	fmt.Fprintf(out, "Survived: %d\n", stats.Survived)
	fmt.Fprintf(out, "Compile Errors: %d\n", stats.CompileError)
	fmt.Fprintf(out, "Errors: %d\n", stats.Error)
	fmt.Fprintf(out, "Timeouts: %d\n", stats.Timeout)
	fmt.Fprintf(out, "Untested: %d\n", stats.Untested)
	fmt.Fprintf(out, "Invalid: %d\n", stats.Invalid)
	fmt.Fprintf(out, "Total: %d\n\n", stats.Total)

	errors := stats.CompileError + stats.Error
	if errors > 0 || stats.Untested > 0 {
		fmt.Fprintf(out, "Error Summary by Operator:\n")
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

		if stats.Untested > 0 {
			fmt.Fprintf(out, "\nUntested by Operator (binary missing - package failed to compile):\n")
			opUntested := make(map[string]int)
			for _, mutant := range mutants {
				if mutant.Status == testing.StatusUntested {
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
			if mutant.Status == testing.StatusError {
				opErrorCount[mutant.Operator.Name()]++
			}
		}
		for op, count := range opErrorCount {
			fmt.Fprintf(out, "  %s: %d errors\n", op, count)
		}
		fmt.Fprintln(out)
	}

	if stats.Killed > 0 {
		fmt.Fprintf(out, "Top Killing Tests:\n")
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
		fmt.Fprintln(out)
	}

	return nil
}
