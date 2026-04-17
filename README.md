## Gorgon v0.6

Go mutation testing tool. Onto version 0.6 now!

Benchmarks: [benchmarks/current_benchmark.txt](benchmarks/current_benchmark.txt)

## Usage

```
gorgon ./path/to/code
gorgon -operators=arithmetic,logical ./path
gorgon -concurrent=all ./path       # use all CPU cores (default)
gorgon -concurrent=half ./path      # use half of CPU cores
gorgon -concurrent=2 ./path         # use exactly 2 concurrent test runners
gorgon -threshold=80 ./path         # fail if mutation score is below 80%
gorgon -cache ./path                # cache results between runs
gorgon -diff HEAD~1 ./path          # only mutate changed lines
gorgon -diff=path/to/file.patch ./path
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-config` | `""` | Path to YAML config file (disables all other flags) |
| `-concurrent` | `all` | Max parallel test runs: `all`, `half`, or a number |
| `-operators` | `all` | Comma-separated operator names or categories |
| `-print-ast` | `false` | Print AST tree and exit |
| `-threshold` | `0` | Fail if mutation score is below this percentage (0-100) |
| `-cache` | `false` | Cache mutation results between runs |
| `-dry-run` | `false` | Preview mutants without running tests |
| `-progbar` | `false` | Show progress percentage during execution |
| `-exclude` | `""` | Comma-separated glob patterns for files to exclude |
| `-include` | `""` | Comma-separated glob patterns for files to include |
| `-skip` | `""` | Comma-separated relative file paths to skip entirely |
| `-skip-func` | `""` | Comma-separated file:function pairs to skip (e.g. foo/bar.go:MyFunc) |
| `-tests` | `""` | Comma-separated relative paths to test files/folders to run |
| `-diff` | `""` | Only mutate changed lines (e.g. HEAD~1, HEAD, or path/to/file.patch) |
| `-debug` | `false` | Show detailed debug output during execution |
| `-show-killed` | `false` | Show killed mutants with test attribution |
| `-show-survived` | `false` | Show survived mutants in output |
| `-format` | `textfile` | Output format for report file (textfile, html, junit, sarif, json) |
| `-output` | `""` | Write report to file (e.g. `report.txt`) |
| `-debug-files` | `false` | Also write debug info to `{output}.debug.txt` |
| `-cpu-profile` | `""` | Write CPU profile to file (analyzable with `go tool pprof`) |
| `-no-regression` | `false` | Fail if mutation score drops below saved baseline |
| `-baseline-file` | `""` | Path to baseline file (default: `.gorgon-baseline.json`) |
| `-baseline-tolerance` | `0` | Allow this many percentage points of score drop before failing |

## Baseline / Ratchet Mode

Large codebases can't jump from 0% to 80% overnight. Baseline mode lets teams improve incrementally without being blocked from day one — the same adoption trick golangci-lint uses.

```
gorgon baseline ./path          # save current score as baseline
gorgon -no-regression ./path    # fail only if score drops from baseline
gorgon -no-regression -baseline-tolerance=1 ./path  # allow 1pp of drift
```

On the first `-no-regression` run with no baseline file, Gorgon auto-saves the current score instead of failing, so teams are never blocked on day one.

### Config

```yaml
baseline:
  no_regression: true
  tolerance: 1.0          # allow 1pp of drift (optional)
  file: ".gorgon-baseline.json"  # override path (optional)
```

The baseline file (`.gorgon-baseline.json`) should be committed to version control so CI can compare against it.

## Config

Use `-config` to load a YAML file. All flags must be omitted when using `-config`.

```yaml
# === Core Mutation Settings ===
operators:
  - all
threshold: 80

# === Execution Settings ===
concurrent: all
cache: true
dry_run: false
progbar: false

# === Test Configuration ===
unit_tests_enabled: true
tests: []

# === External Test Suites ===
external_suites:
  enabled: false
  run_mode: after_unit
  suites: []

# === File Filtering ===
exclude:
  - "*_test.go"
include: []
skip:
  - vendor/
skip_func:
  - foo/bar.go:MyFunc

# === Advanced Options ===
diff: ""
base: ""
debug: false

# === Baseline / Ratchet ===
baseline:
  no_regression: false
  tolerance: 0.0

# === Output Settings ===
show_killed: false
show_survived: false
outputs:
  - textfile:report.txt
  - junit:mutation-results.xml
  - sarif:mutation-results.sarif
  - html:gorgon-report
  - json:mutation-results.json
cpu_profile: ""

# === Directory Rules ===
dir_rules:
  - dir: internal/core
    whitelist:
      - arithmetic_flip
      - boundary_value
  - dir: internal/api
    blacklist:
      - all

# === Suppressions (Auto-managed) ===
suppress: []
```

```
gorgon -config=gorgon.yml ./path
```

## CI Integration

Gorgon supports multiple output formats for CI/CD pipelines and test dashboards.

### Multiple Outputs

Specify all output formats in the config file using the `outputs` list with `format:filepath` pairs:

```yaml
outputs:
  - textfile:report.txt
  - junit:mutation-results.xml
  - sarif:mutation-results.sarif
  - html:gorgon-report
  - json:mutation-results.json
```

All formats are written in a single run. Or via CLI (single format only):
```sh
gorgon -format=junit -output=mutation-results.xml ./path
```

### JUnit XML

For Jenkins, TeamCity, and other CI systems that parse JUnit reports:

```sh
gorgon -format=junit -output=mutation-results.xml ./path
```

Survived mutants appear as test failures, compilation errors as errors, and untested mutants as skipped.

### SARIF

For GitHub Code Scanning and other SARIF-compatible tools:

```sh
gorgon -format=sarif -output=mutation-results.sarif ./path
```

Upload to GitHub Actions:
```yaml
- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v2
  with:
    sarif_file: mutation-results.sarif
```

### HTML

For local review and dashboards:

```sh
gorgon -format=html -output=gorgon-report ./path
```

### JSON

For programmatic consumption and custom tooling:

```sh
gorgon -format=json -output=mutation-results.json ./path
```

Output structure:
```json
{
  "summary": {
    "total": 100,
    "killed": 85,
    "survived": 10,
    "errors": 3,
    "untested": 2,
    "score": 89.47
  },
  "mutants": [
    {
      "id": 1,
      "status": "killed",
      "operator": "arithmetic_flip",
      "file": "pkg/example.go",
      "line": 42,
      "column": 10,
      "killed_by": "TestExample"
    }
  ]
}
```

## External Test Suites

Run black-box tests from external packages (e.g., `/tests/`, `/integration/`) to kill mutations. This allows tests outside the main package to contribute to mutation detection.

### Configuration

Add `external_suites` to your config:

```yaml
unit_tests_enabled: true          # Run unit tests (default: true)
external_suites:
  enabled: true
  run_mode: after_unit            # Options: after_unit, only, alongside
  suites:
    - name: integration
      paths:
        - ./tests/integration
        - ./tests/regression
      tags: [integration]          # Optional: build tags
      short_circuit: true          # Stop on first kill (default: true)
    
    - name: e2e
      paths:
        - ./tests/e2e
      tags: [e2e]
```

### Auto-Discovery

Use glob patterns to automatically discover all test packages:

```yaml
external_suites:
  enabled: true
  suites:
    - name: all-tests
      paths:
        - ./tests/...              # Recursively finds all test packages
```

### Run Modes

- **`after_unit`** (default): Run external suites only on mutants that survived unit tests
- **`only`**: Skip unit tests, run only external suites
- **`alongside`**: Run external suites on all mutants regardless of unit test results

### How It Works

1. **Unit Phase** (if `unit_tests_enabled: true`): Gorgon runs local package tests
2. **External Phase** (if `external_suites.enabled: true`): 
   - Discovers test packages from configured paths
   - Builds test binaries for each package
   - Runs surviving mutants against each binary
   - Mutants killed by external tests are marked with suite name (e.g., `TestName [integration]`)

### Example: Integration Tests Kill Mutations

```go
// examples/mutations/arithmetic_flip/example2.go
package arithmetic_flip

func Example2(a, b int) int {
	return a + b
}
```

```go
// tests/integration/arithmetic_flip_test.go
package testing_test

import (
	"testing"
	"github.com/myorg/myproject/examples/mutations/arithmetic_flip"
)

func TestExample2(t *testing.T) {
	result := arithmetic_flip.Example2(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}
```

When `a + b` is mutated to `a - b`, the external test kills it:
```
Top Killing Tests:
  TestExample2 [integration]                         1 kills
```

### Disabling Unit Tests

To run only external suites:

```yaml
unit_tests_enabled: false
external_suites:
  enabled: true
  run_mode: after_unit
  suites:
    - name: all-tests
      paths: [./tests/...]
```

## Suppressions

Suppress mutations using inline comments or config file entries.

### Inline Comments

Add `//gorgon:ignore` above code to suppress mutations on the next line:

```go
//gorgon:ignore
return "pass"

//gorgon:ignore panic_removal
panic("error")

//gorgon:ignore arithmetic_flip:9
x = a + b
```

### Config File

Add suppressions to your YAML config:

```yaml
suppress:
  - location: path/to/file.go:5
    operators:
      - arithmetic_flip
      - panic_removal
  
  # Omit operators to suppress ALL operators on that line
  - location: path/to/file.go:10
```

Paths are relative to the project root (nearest `go.mod`). Override with `base:`

```yaml
base: examples  # use this dir as root instead of go.mod
suppress:
  - location: mutations/panic_removal/file.go:5
```

### Auto Syncing

When running with `-config`, inline `//gorgon:ignore` comments are automatically added to the config file's `suppress:` section. Each comment becomes a YAML entry with the relative file path, line number, and suppressed operators:

```yaml
suppress:
  - location: pkg/file.go:12
    operators:
      - panic_removal
```

Existing config suppressions are preserved and merged with inline comments. Paths are always relative to the project root, regardless of which subfolder you run Gorgon on.

## Mutations

Arithmetic
- `arithmetic_flip` - + ↔ -, * ↔ /

Logical
- `condition_negation` - == ↔ !=, < ↔ >=, <= ↔ >, > ↔ <=
- `negate_condition` - if (x) → if (!x)
- `logical_operator` - && ↔ ||

Boundary
- `boundary_value` - < ↔ <=, > ↔ >=

Assignment
- `assignment_operator` - = → +=, += ↔ -=, *= ↔ /=

Function Body
- `empty_body` - Replace void function body with {}

Binary Operators
- `binary_math` - % ↔ *, & ↔ |, << ↔ >>
- `inc_dec_flip` - ++ ↔ --
- `sign_toggle` - Unary -x ↔ +x

Literal
- `constant_replacement` - Replace literals with different values
- `variable_replacement` - Replace variable with another of same type
- `zero_value_return_numeric` - Replace numeric literals with 0
- `zero_value_return_string` - Replace string literals with ""
- `zero_value_return_bool` - Replace bool literals with false
- `zero_value_return_error` - Replace fmt.Errorf() with nil

Early Return
- `early_return_removal` - Remove early return statements inside if blocks

Reference Returns
- `pointer_returns` - return &x → return nil
- `slice_returns` - return []T{} → return nil
- `map_returns` - return map[K]V{} → return nil
- `channel_returns` - return make(chan T) → return nil
- `interface_returns` - return "foo" → return nil (interface{} only)

Switch
- `switch_remove_default` - Remove default case from switch
- `swap_case_bodies` - Swap case bodies within same switch

Conditional Expression
- `if_condition_true` - if (a > b) → if (true)
- `if_condition_false` - if (a > b) → if (false)
- `for_condition_true` - for i < 10 {} → for true {}
- `for_condition_false` - for i < 10 {} → for false {}

Loop
- `loop_body_removal` - Remove loop body, leaving empty loop
- `loop_break_first` - Add break after first iteration
- `loop_break_removal` - Remove break statements inside loops

Statement
- `defer_removal` - Remove defer statements

## Engine

- Context-aware: passes type info to mutators
- Extensible: implement Operator or ContextualOperator interface
- Parallel test execution: mutants run concurrently across CPU cores

## Kill Attribution

When a mutant is killed, Gorgon tracks which test detected it:

```
Top Killing Tests:
  TestMainHandlesFlagErrors                        42 kills
  TestMainHandlesValidationErrors                  115 kills

Killed Mutants:
- #12 cmd/gorgon/main.go:34:5 (if_condition_false) killed by TestMainHandlesFlagErrors (12ms)
- #15 cmd/gorgon/main.go:38:9 (negate_condition) killed by TestMainHandlesValidationErrors (8ms)
...
```

Use `-show-killed` or `show_killed: true` in config to display killed mutants. The output includes:
- **Which test** killed the mutant (parsed from `--- FAIL: TestName`)
- **How long** it took to detect (duration from test start to failure)
- **Compiler kills**: mutations that cause compilation failures are also tracked as kills (attributed to `(compiler)`)

## Test Isolation

When `-tests` is specified, Gorgon only tests mutants in the packages covered by those test files. Mutants in other packages are marked as **survived** since no tests target them.

For example, with `tests: [cmd/gorgon/main_test.go]`:
- Mutants in `cmd/gorgon/` are tested (the tests target this package)
- Mutants in `pkg/config/`, `examples/`, etc. are marked **survived** (no tests cover them)

This prevents false "kill" counts where tests appear to kill mutants they don't actually test.

## Diff Filtering

Use `-diff` to only mutate lines that have changed since a specific git reference or patch file:

```
gorgon -diff HEAD~1 ./path      # last commit
gorgon -diff HEAD ./path        # staged changes
gorgon -diff main ./path        # divergence from main branch
gorgon -diff abc1234 ./path     # specific commit SHA
gorgon -diff=path/to/file.patch ./path
```

This is useful for CI/CD pipelines to focus mutation testing on changed code only.

### Config

```yaml
diff: "HEAD~1"
```

When `-config` is used, inline `//gorgon:ignore` comments are automatically added to the config file's `suppress:` section. Each comment becomes a YAML entry with the relative file path, line number, and suppressed operators:

```yaml
suppress:
  - location: pkg/file.go:12
    operators:
      - panic_removal
```

Existing config suppressions are preserved and merged with inline comments. Paths are always relative to the project root, regardless of which subfolder you run Gorgon on.

## Per-Directory Operator Rules

Use `dir_rules` in your config file to control which operators apply to specific directories. This is useful for enforcing different mutation testing policies across different parts of your codebase.

### Whitelist

Only allow specific operators in a directory:

```yaml
dir_rules:
  - dir: internal/core
    whitelist:
      - arithmetic_flip
      - boundary_value
      - condition_negation
```

### Blacklist

Block specific operators in a directory:

```yaml
dir_rules:
  - dir: internal/api
    blacklist:
      - defer_removal
      - concurrency
```

### Exclude Entire Directory

Use `all` as a blacklist value to skip an entire directory:

```yaml
dir_rules:
  - dir: vendor/
    blacklist:
      - all
```

### How It Works

- Rules match by directory prefix (longest match wins)
- Whitelist takes precedence over blacklist
- If no rule matches, all operators apply
- `dir_rules` is config-file-only (not a CLI flag)

### Example Config

```yaml
dir_rules:
  - dir: internal/core
    whitelist:
      - arithmetic_flip
      - math_operators
      - boundary_value
  - dir: internal/api
    blacklist:
      - all
  - dir: pkg/config
    blacklist:
      - concurrency
      - defer_removal
```

## Per-Package / Per-Module Configuration Overrides

This is a meaningful feature for monorepos. Sub-configs are named `gorgon.yml` and discovered during a tree walk. The root config is identified by being explicitly passed via `-config`; everything else found during the tree walk is a sub-config.

### Discovery Model

- Sub-configs are named `gorgon.yml` — same filename as the root
- Discovery skips `vendor/`, `.git/`, and `_`-prefixed directories
- Chaining behavior: root → core/gorgon.yml → core/auth/gorgon.yml for a file in core/auth/
- Each level in the chain applies in order

### What's Overrideable

**Replace semantics** (deepest sub-config wins outright):
- `operators` — core risk profile decision, a subtree owns this entirely
- `threshold` — different quality bars per package (generated code vs. core library)
- `tests` — a subtree may have its own test suite or integration tests
- `concurrent` — a subtree with expensive tests may want to throttle parallelism

**Merge/additive semantics** (all levels in the chain contribute):
- `exclude` / `include` — accumulated; deeper configs add more file filters
- `skip` / `skip_func` — accumulated; you never want a parent to un-skip something a child skipped
- `suppress` — accumulated; suppressions only grow as you go deeper
- `dir_rules` — accumulated; deeper rules are more specific and evaluated with existing longest-prefix logic

**Not overrideable** (global only):
- `cache`, `dry_run`, `debug`, `progbar` — run-mode flags
- `format`, `output`, `cpu_profile` — output concerns, single report
- `diff` — a global VCS filter
- `base` — structural, set once

### How It Works

1. **Discovery**: Gorgon walks the project tree looking for `gorgon.yml` files (excluding `vendor/`, `.git/`, `_`-prefixed dirs)
2. **Chaining**: For each file, Gorgon builds a chain of all sub-configs that are ancestors-or-self
3. **Resolution**: 
   - `operators`, `threshold`, `tests`, `concurrent`: deepest sub-config wins
   - `exclude`, `include`, `skip`, `skip_func`, `suppress`, `dir_rules`: accumulated across all levels

### Example

```yaml
# Root config (gorgon.yml at project root)
threshold: 80
operators:
  - all

# In internal/core/gorgon.yml
threshold: 90
operators:
  - arithmetic_flip
  - boundary_value

# In internal/api/gorgon.yml
threshold: 70
concurrent: 2
```

For a file in `internal/api/handler.go`:
- Uses `threshold: 70` (from api sub-config)
- Uses `operators: [all]` (from root, since api doesn't specify operators)
- Uses `concurrent: 2` (from api sub-config)

### Per-Package Threshold Checking

When sub-configs are present, mutation score thresholds are checked per-package. Each package can have its own threshold defined in its sub-config. The reporter will show which packages failed their threshold check:

```
Packages below threshold:
 pkg/testdata/subconfig: 85.00% (threshold 90.00%)
```

### Logging

When sub-configs are discovered, Gorgon logs the count:
```
Loaded sub-configs from 3 directories
```

In debug mode, additional details about operator filtering per directory are shown:
```
[DEBUG] Dir rule internal/core: whitelist 3 operators for internal/core/handler.go
```

### Important Notes

- Sub-configs work with or without `go.mod` files in subdirectories
- The `operators` field in a sub-config applies to ALL files in that directory subtree
- To have different operators for nested directories, create separate `gorgon.yml` files in each nested directory
- `dir_rules` in a sub-config provides fine-grained control WITHIN that directory's subtree

### Sub-Config File Format

A sub-config file (`gorgon.yml`) in a subdirectory can contain any of the overrideable fields:

```yaml
# Example: internal/core/gorgon.yml
threshold: 90
operators:
  - arithmetic_flip
  - boundary_value
  - condition_negation
concurrent: 2
tests:
  - internal/core/core_test.go
exclude:
  - '*_generated.go'
skip:
  - internal/core/legacy/
skip_func:
  - internal/core/legacy/legacy.go:OldFunc
suppress:
  - location: internal/core/legacy/legacy.go:42
    operators:
      - arithmetic_flip
dir_rules:
  - dir: internal/core/internal
    blacklist:
      - all
```

### Sub-Config Precedence

When multiple sub-configs apply to a file (e.g., root → core → core/auth), the precedence is:

1. **For replace fields** (`operators`, `threshold`, `tests`, `concurrent`):
   - Deepest sub-config wins
   - If a field is not set in a sub-config, it falls back to the parent

2. **For merge fields** (`exclude`, `include`, `skip`, `skip_func`, `suppress`, `dir_rules`):
   - All sub-configs contribute
   - Order: root → deeper → deepest
   - Later entries are appended to earlier ones

3. **For dir_rules specifically**:
   - Rules from all levels are accumulated
   - Longest-prefix matching still applies within the merged set
   - Whitelist takes precedence over blacklist

### Example: Nested Sub-Configs

```
project/
├── gorgon.yml              # root: threshold=80, operators=[all]
├── internal/
│   ├── gorgon.yml          # internal: threshold=90, operators=[arithmetic_flip]
│   └── core/
│       ├── gorgon.yml      # core: threshold=95, operators=[boundary_value]
│       └── handler.go      # Uses: threshold=95, operators=[boundary_value]
```

For `internal/core/handler.go`:
- `threshold`: 95 (from core sub-config)
- `operators`: [boundary_value] (from core sub-config)
- `exclude`, `skip`, etc.: accumulated from all three configs

### Deep Dive: How Sub-Configs Are Resolved

#### 1. Discovery Phase

Gorgon walks the directory tree starting from the project root:

```
project/
├── gorgon.yml              # Root config (explicitly passed via -config)
├── internal/
│   ├── gorgon.yml          # Sub-config #1
│   └── core/
│       ├── gorgon.yml      # Sub-config #2
│       └── handler.go
```

Each `gorgon.yml` found (except the root config) is stored as a sub-config entry with its directory path.

#### 2. Chain Building

For each file being mutated, Gorgon builds a chain of applicable sub-configs:

```
File: project/internal/core/handler.go

Chain (shallowest → deepest):
1. project/internal/gorgon.yml
2. project/internal/core/gorgon.yml
```

The chain includes all sub-configs that are ancestors of the file's directory.

#### 3. Field Resolution

**Replace fields** (last one wins):
- `operators`: Walk chain deepest→shallowest, return first non-empty list
- `threshold`: Walk chain deepest→shallowest, return first non-nil pointer
- `tests`: Walk chain deepest→shallowest, return first non-empty list
- `concurrent`: Walk chain deepest→shallowest, return first non-empty string

**Merge fields** (accumulate all):
- `exclude`: root.Exclude + chain[0].Exclude + chain[1].Exclude + ...
- `include`: root.Include + chain[0].Include + chain[1].Include + ...
- `skip`: root.Skip + chain[0].Skip + chain[1].Skip + ...
- `skip_func`: root.SkipFunc + chain[0].SkipFunc + chain[1].SkipFunc + ...
- `suppress`: root.Suppress + chain[0].Suppress + chain[1].Suppress + ...
- `dir_rules`: root.DirRules + chain[0].DirRules + chain[1].DirRules + ...

#### 4. Operator Application

For each mutation site:

1. Start with root operators (or all operators if root has `all`)
2. Apply sub-config operator override (replace semantics)
3. Apply dir_rules filtering (merge semantics from all levels)
4. Generate mutants with the final operator list

#### 5. Threshold Checking

When reporting results:

1. Group mutants by their package directory
2. For each package, look up the effective threshold from the chain
3. Calculate mutation score for that package
4. Check if score meets the package's threshold
5. Report any packages that failed their threshold

## Output Files

Write the report to a file instead of (or in addition to) stdout:

```
gorgon -output=report.txt ./path
```

This writes the full report (mutation score, top killers, killed/survived mutants) to `report.txt` while still printing to stdout.

### Debug Files

Enable `-debug-files` to also write detailed debug information:

```
gorgon -output=report.txt -debug-files ./path
```

This creates two files:
- `report.txt` — the standard report (stats, killed mutants, survived mutants)
- `report.debug.txt` — detailed debug info (error summaries, per-mutant compilation errors)

### Config

```yaml
format: textfile
output: "report.txt"
debug_files: true
```

Currently `textfile` and `html` format is supported.

## CPU Profiling

Use `-cpu-profile=file.out` or `cpu_profile: "file.out"` to write a CPU profile. Analyze it with:

```
go tool pprof -http=:8080 file.out
```

Use `cpu_profile: "true"` to write to `gorgon.cpuprofile` in the current directory.
