package testing

import (
	"testing"
)

func TestParseFailedTest(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "parses FAIL line",
			input: "--- FAIL: TestFoo (0.01s)\nsome output\n",
			want:  "TestFoo",
		},
		{
			name:  "empty output falls back",
			input: "",
			want:  "(compilation/runtime error)",
		},
		{
			name:  "non-empty output without FAIL line",
			input: "some random output",
			want:  "(test output non-empty)",
		},
		{
			name:  "multiple FAIL lines takes first",
			input: "--- FAIL: TestFirst (0.01s)\n--- FAIL: TestSecond (0.02s)\n",
			want:  "TestFirst",
		},
		{
			name:  "FAIL line with package prefix",
			input: "--- FAIL: github.com/foo/bar.TestFoo (0.01s)\n",
			want:  "github.com/foo/bar.TestFoo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFailedTest(tc.input)
			if got != tc.want {
				t.Errorf("parseFailedTest(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAbsInt(t *testing.T) {
	cases := []struct {
		name string
		x    int
		want int
	}{
		{name: "positive", x: 5, want: 5},
		{name: "negative", x: -5, want: 5},
		{name: "zero", x: 0, want: 0},
		{name: "large positive", x: 1000000, want: 1000000},
		{name: "large negative", x: -1000000, want: 1000000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := absInt(tc.x)
			if got != tc.want {
				t.Errorf("absInt(%d) = %d, want %d", tc.x, got, tc.want)
			}
		})
	}
}

func TestIsCompilationError(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "compilation failed", input: "compilation failed: some error", want: true},
		{name: "build failed", input: "build failed due to syntax error", want: true},
		{name: "undefined variable", input: "undefined: foo", want: true},
		{name: "syntax error", input: "syntax error: unexpected }", want: true},
		{name: "mismatched types", input: "mismatched types int and string", want: true},
		{name: "cannot assign", input: "cannot assign int to string", want: true},
		{name: "regular test output", input: "--- FAIL: TestFoo (0.01s)\n    assertion failed", want: false},
		{name: "empty output", input: "", want: false},
		{name: "pass output", input: "PASS\nok package", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isCompilationError(tc.input)
			if got != tc.want {
				t.Errorf("isCompilationError(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestExtractFirstError(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple error", input: "error: something went wrong", want: "error: something went wrong"},
		{name: "first line is content", input: "first line\nerror: second line\nline3", want: "first line"},
		{name: "with indentation", input: "  error: indented error", want: "error: indented error"},
		{name: "no error keyword", input: "some warning message", want: "some warning message"},
		{name: "empty string", input: "", want: "unknown error"},
		{name: "skip special prefixes", input: "# comment\nactual error", want: "actual error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractFirstError(tc.input)
			if got != tc.want {
				t.Errorf("extractFirstError(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestExtractErrorType(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "undefined identifier", input: "undefined: someVariable", want: "undefined: undefined: someVariable"},
		{name: "syntax error", input: "syntax error: unexpected }", want: "syntax error: syntax error: unexpected }"},
		{name: "mismatched types", input: "mismatched types foo", want: "type mismatch: mismatched types foo"},
		{name: "generic error", input: "some random error", want: "some random error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractErrorType(tc.input)
			if got != tc.want {
				t.Errorf("extractErrorType(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestHasNoTestsToRun(t *testing.T) {
	// This function requires a non-nil *exec.ExitError with specific patterns in stderr
	// For unit testing, we can only test the nil error case which returns false
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasNoTestsToRun("", tc.err)
			if got != tc.want {
				t.Errorf("hasNoTestsToRun(_, %v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSumMutantIDs(t *testing.T) {
	cases := []struct {
		name string
		m    map[string][]int
		want int
	}{
		{
			name: "empty map",
			m:    map[string][]int{},
			want: 0,
		},
		{
			name: "single package",
			m:    map[string][]int{"pkg1": {1, 2, 3}},
			want: 3,
		},
		{
			name: "multiple packages",
			m:    map[string][]int{"pkg1": {1, 2}, "pkg2": {3, 4, 5}},
			want: 5,
		},
		{
			name: "empty package list",
			m:    map[string][]int{"pkg1": {}, "pkg2": {1}},
			want: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sumMutantIDs(tc.m)
			if got != tc.want {
				t.Errorf("sumMutantIDs(%v) = %d, want %d", tc.m, got, tc.want)
			}
		})
	}
}

func TestNewProgressTracker(t *testing.T) {
	pt := NewProgressTracker(10)
	if pt.total != 10 {
		t.Errorf("NewProgressTracker(10).total = %d, want 10", pt.total)
	}
	if pt.done != 0 {
		t.Errorf("NewProgressTracker(10).done = %d, want 0", pt.done)
	}
}

func TestProgressTrackerRecord(t *testing.T) {
	pt := NewProgressTracker(5)
	pt.Record()
	if pt.done != 1 {
		t.Errorf("ProgressTracker.Record() did not increment done counter")
	}
}

func TestProgressTrackerFinish(t *testing.T) {
	// Finish() doesn't set done to total, it just prints final progress
	// This test verifies Finish() doesn't panic
	pt := NewProgressTracker(5)
	pt.Finish()
	// If we got here without panic, test passes
}

func TestTestArgs(t *testing.T) {
	cases := []struct {
		name      string
		timeout   string
		tests     []string
		wantLen   int
		wantFirst string
	}{
		{name: "empty timeout", timeout: "", tests: []string{}, wantLen: 1, wantFirst: "-test.timeout=5s"},
		{name: "zero timeout", timeout: "0s", tests: []string{}, wantLen: 1, wantFirst: "-test.timeout=5s"},
		{name: "with timeout", timeout: "10s", tests: []string{}, wantLen: 1, wantFirst: "-test.timeout=10s"},
		{name: "with tests", timeout: "5s", tests: []string{"TestFoo"}, wantLen: 2, wantFirst: "-test.timeout=5s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := testArgs(tc.timeout, tc.tests)
			if len(got) != tc.wantLen {
				t.Errorf("testArgs(%q, %v) returned %d args, want %d", tc.timeout, tc.tests, len(got), tc.wantLen)
			}
			if len(got) > 0 && got[0] != tc.wantFirst {
				t.Errorf("testArgs(%q, %v)[0] = %q, want %q", tc.timeout, tc.tests, got[0], tc.wantFirst)
			}
		})
	}
}

