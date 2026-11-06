package testing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runResult is the structured outcome of running one mutant against the
// prebuilt test binary. Derived from the test framework's documented verbose
// output protocol — no error-message substring matching.
type runResult struct {
	status     string
	killedBy   string
	killOutput string
	duration   time.Duration
	sawRun     bool
}

// runTestBinary executes a prebuilt go test binary directly. The binary is
// invoked with -test.v so the test framework emits the documented event lines
// ("=== RUN", "--- PASS:", "--- FAIL:") used by classifyVerboseRun.
func runTestBinary(ctx context.Context, binary, dir string, env []string, testFilter string, testTimeout time.Duration) ([]byte, error) {
	args := []string{"-test.v"}
	if testTimeout > 0 {
		args = append(args, "-test.timeout="+testTimeout.String())
	}
	if testFilter != "" {
		args = append(args, "-test.run="+testFilter)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}

func envWithMutant(id int) []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "GORGON_MUTANT_ID=") {
			continue
		}
		env = append(env, kv)
	}
	if id != 0 {
		env = append(env, fmt.Sprintf("GORGON_MUTANT_ID=%d", id))
	}
	return env
}

// Documented `go test -v` line prefixes. These have been part of the testing
// package's output contract since Go 1.0.
//
//	=== RUN   <name>
//	--- PASS: <name> (<duration>)
//	--- FAIL: <name> (<duration>)
//	--- SKIP: <name> (<duration>)
const (
	prefixRUN      = "=== RUN   "
	prefixFAIL     = "--- FAIL: "
	timeoutPanic   = "panic: test timed out after"
	noTestsWarning = "testing: warning: no tests to run"
)

//

func classifyVerboseRun(output []byte, runErr error, deadlineExceeded bool, baselineRan bool) runResult {
	firstFail := ""
	sawRun := false
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		s := strings.TrimLeft(string(line), " \t")
		if !sawRun && strings.HasPrefix(s, prefixRUN) {
			sawRun = true
		}
		if firstFail == "" && strings.HasPrefix(s, prefixFAIL) {
			firstFail = parseFailLineName(s)
		}
	}

	if deadlineExceeded || bytes.Contains(output, []byte(timeoutPanic)) {
		return runResult{status: StatusTimeout, killedBy: "(timeout)", killOutput: truncOutput(output), sawRun: sawRun}
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return runResult{
				status:     StatusError,
				killedBy:   "",
				killOutput: fmt.Sprintf("test binary could not be run: %v", runErr),
				sawRun:     sawRun,
			}
		}
		killedBy := firstFail
		if killedBy == "" {
			if sawRun {
				killedBy = "(test binary crashed)"
			} else {
				killedBy = "(crashed before running tests: init or TestMain)"
			}
		}
		return runResult{status: StatusKilled, killedBy: killedBy, killOutput: truncOutput(output), sawRun: sawRun}
	}

	if !sawRun {
		if baselineRan {
			return runResult{status: StatusSurvived, killOutput: "no test ran under this mutant (the baseline ran tests); the suite still passed", sawRun: false}
		}
		return runResult{status: StatusUntested, sawRun: false}
	}
	return runResult{status: StatusSurvived, sawRun: true}
}

// parseFailLineName extracts the test name from a documented FAIL line of the
// form "--- FAIL: TestName (0.01s)". The name terminates at the first space
// before the duration parenthesis — the test framework guarantees this layout.
func parseFailLineName(line string) string {
	rest := line[len(prefixFAIL):]
	if idx := strings.Index(rest, " ("); idx > 0 {
		return rest[:idx]
	}
	// Defensive: if the duration suffix is missing, take everything up to the
	// next whitespace.
	if idx := strings.IndexAny(rest, " \t"); idx > 0 {
		return rest[:idx]
	}
	return rest
}

func truncOutput(b []byte) string {
	const maxOut = 2000
	if len(b) > maxOut {
		return string(b[:maxOut])
	}
	return string(b)
}

func failingTests(output []byte) []string {
	var names []string
	seen := make(map[string]bool)
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		s := string(line)
		if !strings.HasPrefix(s, prefixFAIL) {
			continue
		}
		name := parseFailLineName(s)
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// packageHasGoTestFiles asks `go list` whether the given package import-path
// contains any in-package or external test files. It respects build tags, so
// a package whose only test files are gated behind a tag the run isn't using
// is correctly reported as having no tests.
//
// importPath is interpreted relative to dir (typically a "./..." pattern).
func packageHasGoTestFiles(ctx context.Context, dir, importPath string, buildTags []string) (bool, error) {
	args := []string{"list", "-f", "{{len .TestGoFiles}}+{{len .XTestGoFiles}}"}
	if len(buildTags) > 0 {
		args = append(args, "-tags", strings.Join(buildTags, ","))
	}
	args = append(args, importPath)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("go list %s: %w (%s)", importPath, err, strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(string(out))
	plus := strings.Index(line, "+")
	if plus < 0 {
		return false, fmt.Errorf("unexpected go list output: %q", line)
	}
	return line[:plus] != "0" || line[plus+1:] != "0", nil
}
