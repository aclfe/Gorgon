package testing

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runResult is the structured outcome of running one mutant against the
// prebuilt test binary. Derived from the test framework's documented verbose
// output protocol — no error-message substring matching.
type runResult struct {
	status     string // "killed" | "survived" | "timeout" | "untested" | "error"
	killedBy   string
	killOutput string
}

// runTestBinary executes a prebuilt go test binary directly. The binary is
// invoked with -test.v so the test framework emits the documented event lines
// ("=== RUN", "--- PASS:", "--- FAIL:") used by classifyVerboseRun.
func runTestBinary(ctx context.Context, binary, dir string, env []string, testFilter, testTimeout string) ([]byte, error) {
	args := []string{"-test.v"}
	if testTimeout != "" {
		args = append(args, "-test.timeout="+testTimeout)
	}
	if testFilter != "" {
		args = append(args, "-test.run="+testFilter)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd.CombinedOutput()
}

// Documented `go test -v` line prefixes. These have been part of the testing
// package's output contract since Go 1.0.
//
//	=== RUN   <name>
//	--- PASS: <name> (<duration>)
//	--- FAIL: <name> (<duration>)
//	--- SKIP: <name> (<duration>)
const (
	prefixRUN  = "=== RUN   "
	prefixFAIL = "--- FAIL: "
)

// classifyVerboseRun consumes the binary's stdout+stderr and the exit error
// and produces a deterministic mutant status. Rules:
//
//   - context deadline exceeded                -> timeout
//   - any "--- FAIL: <name>" line              -> killed (first such name)
//   - at least one "=== RUN " line, exit==0    -> survived
//   - at least one "=== RUN " line, exit!=0    -> killed ("runtime error")
//     (test ran, then the process crashed in a goroutine)
//   - no "=== RUN " line, exit==0              -> untested
//   - no "=== RUN " line, exit!=0              -> error (init/TestMain crash)
func classifyVerboseRun(output []byte, runErr error, deadlineExceeded bool) runResult {
	if deadlineExceeded {
		return runResult{
			status:     "timeout",
			killedBy:   "(timeout)",
			killOutput: "test timed out",
		}
	}

	firstFail := ""
	sawRun := false
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		s := string(line)
		// Allow leading whitespace from subtests indented by Go's test runner.
		s = strings.TrimLeft(s, " \t")
		if !sawRun && strings.HasPrefix(s, prefixRUN) {
			sawRun = true
		}
		if firstFail == "" && strings.HasPrefix(s, prefixFAIL) {
			firstFail = parseFailLineName(s)
		}
	}

	if firstFail != "" {
		return runResult{
			status:     "killed",
			killedBy:   firstFail,
			killOutput: truncOutput(output),
		}
	}

	if !sawRun {
		if runErr != nil {
			return runResult{
				status:     "error",
				killedBy:   "runtime error",
				killOutput: truncOutput(output),
			}
		}
		return runResult{status: "untested"}
	}

	if runErr != nil {
		return runResult{
			status:     "killed",
			killedBy:   "runtime error",
			killOutput: truncOutput(output),
		}
	}

	return runResult{status: "survived"}
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
	const maxOut = 300
	if len(b) > maxOut {
		return string(b[:maxOut])
	}
	return string(b)
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
