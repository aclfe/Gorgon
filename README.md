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

# Go workspaces (go.work) are automatically detected
gorgon ./...                        # tests all modules in workspace
```

### Flags

The CLI surface is intentionally small. Filtering, output, baseline, profiling, and policy settings live in `gorgon.yml` so they version with the project.

| Flag | Default | Description |
|---|---|---|
| `-config` | `""` | Path to YAML config file. Cannot be combined with other flags |
| `-pkg` | `.` | Package path to mutate (overridable by positional targets) |
| `-operators` | `all` | Comma-separated operator names or categories |
| `-concurrent` | `all` | Max parallel test runs: `all`, `half`, or a number |
| `-threshold` | `0` | Fail if mutation score is below this percentage (0-100) |
| `-cache` | `false` | Cache mutation results between runs |
| `-dry-run` | `false` | Preview mutants without running tests |
| `-diff` | `""` | Only mutate changed lines (e.g. `HEAD~1`, a commit SHA, or `path/to/file.patch`) |
| `-progbar` | `false` | Show progress percentage during execution |
| `-show-killed` | `false` | Show killed mutants with test attribution |
| `-show-survived` | `false` | Show survived mutants in output |
| `-debug` | `false` | Enable full debug output (also writes `{output}.debug.txt` when an `outputs:` textfile is configured) |
| `-print-ast` | `false` | Print AST tree and exit |
| `-mem-profile` | `""` | Write periodic heap profiles to this directory (e.g. `profiles`) |

Settings that exist only in the config file (no CLI flag): `exclude`, `include`, `skip`, `skip_func`, `tests`, `outputs`, `cpu_profile`, `mem_profile`, `badge`, `baseline.*`, `external_suites.*`, `dir_rules`, `suppress`, `go_version`, `chunk_large_files`, `build_tags`, `sub_config_mode`, `threshold_inherit`, `violation_mode`, `unit_tests_enabled`, `base`.

## Baseline / Ratchet Mode

Large codebases can't jump from 0% to 80% overnight. Baseline mode lets teams improve incrementally without being blocked from day one — the same adoption trick golangci-lint uses.

Baseline is configured in `gorgon.yml`:

```yaml
baseline:
  no_regression: true            # fail only if score drops from saved baseline
  tolerance: 1.0                 # allow 1pp of drift (optional)
  file: ".gorgon-baseline.json"  # override path (optional)
  save: false                    # set true to overwrite the baseline this run
```

```
gorgon -config=gorgon.yml ./path
```

On the first run with `no_regression: true` and no baseline file present, Gorgon auto-saves the current score instead of failing, so teams are never blocked on day one. To deliberately re-baseline, set `baseline.save: true` for one run.

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
go_version: ""  # Override detected Go version (e.g., "1.25", "1.26")
chunk_large_files: true  # Split files with >500 mutants to reduce memory (default: true)
build_tags: []  # Tags forwarded to `go test -c` (e.g. ["unit", "integration"])

# === Sub-Config / Policy Behavior ===
sub_config_mode: merge          # merge (default), replace, or isolate
threshold_inherit: false        # propagate root threshold to sub-configs without their own
violation_mode: fail            # fail (default), warn, or silent — for org-policy violations

# === Baseline / Ratchet ===
baseline:
  no_regression: false
  tolerance: 0.0
  save: false

# === Output Settings ===
show_killed: false
show_survived: false
outputs:
  - textfile:report.txt
  - junit:mutation-results.xml
  - sarif:mutation-results.sarif
  - html:gorgon-report
  - json:mutation-results.json
cpu_profile: ""        # Write CPU profile to this path (or "true" → ./gorgon.cpuprofile)
mem_profile: ""        # Write periodic heap profiles to this directory
badge: ""              # Generate badge: "json" or "svg"

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

## Badge Generation

Generate shields.io-compatible badges to display mutation score in your README.

### Configuration

Add to your `gorgon.yml`:

```yaml
badge: json  # or "svg"
```

When you run Gorgon, it will automatically generate the badge file in your project directory.

### Generate JSON Badge

```yaml
# gorgon.yml
badge: json
outputs:
  - textfile:report.txt
```

```bash
gorgon -config=gorgon.yml ./...
# Creates: mutation-badge.json
```

Output:
```json
{
  "schemaVersion": 1,
  "label": "mutation",
  "message": "85.3%",
  "color": "#4c1"
}
```

Host this JSON file and use it with shields.io:
```markdown
![Mutation Score](https://img.shields.io/endpoint?url=https://your-site.com/mutation-badge.json)
```

### Generate SVG Badge

```yaml
# gorgon.yml
badge: svg
```

```bash
gorgon -config=gorgon.yml ./...
# Creates: mutation-badge.svg
```

Commit the SVG to your repo and reference it:
```markdown
![Mutation Score](./mutation-badge.svg)
```

### Badge Colors

- **Green** (≥80%): `#4c1`
- **Yellow-Green** (≥60%): `#97ca00`
- **Yellow** (≥40%): `#dfb317`
- **Orange** (≥20%): `#fe7d37`
- **Red** (<20%): `#e05d44`

## GitHub Actions

### Quick Setup (2 lines)

Add to `.github/workflows/mutation-test.yml`:

```yaml
- name: Run Mutation Testing
  uses: gorgon/gorgon-action@v1
```

That's it! The action automatically:
- Installs Gorgon
- Runs mutation testing
- Uploads badge and reports as artifacts
- Fails the build if threshold not met

### Full Configuration

```yaml
name: Mutation Testing

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  mutation-test:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Gorgon
        uses: gorgon/gorgon-action@v1
        with:
          config: 'gorgon.yml'          # Optional: path to config
          targets: './...'               # Optional: target paths
          threshold: '70'                # Optional: minimum score
          fail-on-threshold: 'true'      # Optional: fail build
          upload-badge: 'true'           # Optional: upload badge
          upload-reports: 'true'         # Optional: upload reports
      
      - name: Comment PR
        if: github.event_name == 'pull_request'
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const report = fs.readFileSync('gorgon-report.txt', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.name,
              body: `## 🧬 Mutation Testing Results\n\n\`\`\`\n${report}\n\`\`\``
            });
```

### Action Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `version` | Gorgon version to use | `latest` |
| `config` | Path to gorgon.yml | `""` |
| `targets` | Target paths (space-separated) | `./...` |
| `threshold` | Minimum mutation score (0-100) | `0` |
| `fail-on-threshold` | Fail build if threshold not met | `true` |
| `upload-badge` | Upload badge as artifact | `true` |
| `upload-reports` | Upload reports as artifacts | `true` |

### Action Outputs

| Output | Description |
|--------|-------------|
| `mutation-score` | The mutation score percentage |
| `total-mutants` | Total number of mutants |
| `killed` | Number of killed mutants |
| `survived` | Number of survived mutants |

### Using Outputs

```yaml
- name: Run Gorgon
  id: gorgon
  uses: gorgon/gorgon-action@v1

- name: Check Score
  run: |
    echo "Mutation Score: ${{ steps.gorgon.outputs.mutation-score }}%"
    echo "Killed: ${{ steps.gorgon.outputs.killed }}/${{ steps.gorgon.outputs.total }}"
```

### Artifacts

The action uploads two artifacts:

1. **mutation-badge** - JSON and SVG badge files
2. **mutation-reports** - Text report and baseline file

Download from the Actions tab or use in subsequent steps.

## CI Integration

Gorgon supports multiple output formats for CI/CD pipelines and test dashboards. Output formats are configured in `gorgon.yml` via the `outputs:` list of `format:filepath` pairs — there is no CLI flag for format/output.

### Multiple Outputs

```yaml
outputs:
  - textfile:report.txt
  - junit:mutation-results.xml
  - sarif:mutation-results.sarif
  - html:gorgon-report
  - json:mutation-results.json
```

All requested formats are written in a single run.

### JUnit XML

For Jenkins, TeamCity, and other CI systems that parse JUnit reports:

```yaml
outputs:
  - junit:mutation-results.xml
```

Survived mutants appear as test failures, compilation errors as errors, and untested mutants as skipped.

### SARIF

For GitHub Code Scanning and other SARIF-compatible tools:

```yaml
outputs:
  - sarif:mutation-results.sarif
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

```yaml
outputs:
  - html:gorgon-report
```

### JSON

For programmatic consumption and custom tooling:

```yaml
outputs:
  - json:mutation-results.json
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

- **Context-aware**: contextual operators receive the enclosing function's `ReturnType` (comma-separated for multi-value returns), `EnclosingFunc`, file AST, and node parent.
- **Extensible**: implement `Operator` for AST-only mutations, `ContextualOperator` to use the surrounding context, or `SafetyConstrainedOperator` to declare site shapes the operator should never produce. New operators self-register via `init()` — add a blank import in `cmd/gorgon/main.go` and no other wiring is needed.
- **Three-level preflight before build**: L1 nil/contract checks, L2 schemata AST integrity, L3 module-aware type-check via `golang.org/x/tools/go/packages` (no stub importer — unresolved imports surface as real errors).
- **Schemata-based execution**: every mutant in a file is inlined into one transformed copy guarded by `if activeMutantID == N { mutated } else { original }`. One `go test -c` compile serves all mutants in a package.
- **Deterministic kill classification**: mutant runs are classified from the test framework's documented `=== RUN` / `--- FAIL:` verbose output protocol, not heuristic substring matches. "No tests" is determined by `go list -f '{{len .TestGoFiles}}+{{len .XTestGoFiles}}'`.
- **Parallel test execution**: mutants run concurrently across CPU cores; per-package compile and per-mutant test runs are scheduled on separate worker pools.

### Operator Safety Contract

Operators that can prove a site shape will never type-check should implement `SafetyConstrainedOperator`:

```go
type SafetyConstrainedOperator interface {
    Operator
    // IsAlwaysInvalidFor reports whether applying this operator at any site
    // whose enclosing function has the given returnType signature is
    // statically guaranteed to produce an invalid mutant. returnType is the
    // engine's comma-separated form, e.g. "int,error".
    IsAlwaysInvalidFor(returnType string) bool
}
```

Implementing it lets preflight L1 reject those mutants before any AST surgery, instead of burning a compile cycle on each. Operators that don't implement it are passed through to L3 type-check, which is the authoritative validator.

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

## Go Workspace Support

Gorgon fully supports Go workspaces (`go.work`) for multi-module monorepos. Workspace detection is automatic — no configuration required.

### How It Works

When you run Gorgon on a directory, it:

1. **Searches for `go.work`** by walking up the directory tree
2. **Falls back to `go.mod`** if no workspace is found (single-module mode)
3. **Enumerates all workspace members** from `use` directives
4. **Copies all modules** to the temporary test environment
5. **Preserves cross-module dependencies** via `go.work` and `go.work.sum`

### Example Workspace Layout

```
monorepo/
├── go.work              # Workspace root
├── go.work.sum
├── gorgon.yml           # Optional: root config
├── service-a/
│   ├── go.mod
│   ├── go.sum
│   ├── gorgon.yml       # Optional: service-a specific config
│   └── pkg/
│       └── handler.go
├── service-b/
│   ├── go.mod
│   ├── go.sum
│   ├── gorgon.yml       # Optional: service-b specific config
│   └── pkg/
│       └── api.go
└── shared/
    ├── go.mod
    └── pkg/
        └── common.go
```

**go.work:**
```go
go 1.25

use ./service-a
use ./service-b
use ./shared
```

### Running Gorgon on a Workspace

```bash
# From workspace root - tests all modules
gorgon ./...

# Target specific module
gorgon ./service-a

# Target multiple modules
gorgon ./service-a ./service-b

# With config (discovers sub-configs in all modules)
gorgon -config=gorgon.yml ./...
```

### Cross-Module Imports

Workspace mode preserves cross-module dependencies. If `service-a` imports `shared`, mutations in `shared` are correctly tested by `service-a`'s tests.

```go
// service-a/pkg/handler.go
import "monorepo/shared/pkg"

func Handle() {
    return shared.Common() // mutation in shared.Common() is tested
}
```

### Sub-Configs in Workspaces

Each workspace member can have its own `gorgon.yml`:

```yaml
# service-a/gorgon.yml
threshold: 90
operators:
  - arithmetic_flip
  - boundary_value

# service-b/gorgon.yml  
threshold: 70
operators:
  - all
```

Sub-config discovery walks **all workspace members**, not just the workspace root. See [Per-Package Configuration](#per-package--per-module-configuration-overrides) for details on sub-config resolution.

### Single-Module Compatibility

Projects without `go.work` continue to work exactly as before:

```
project/
├── go.mod
├── go.sum
└── pkg/
    └── code.go
```

```bash
gorgon ./...  # Works identically to v0.5
```

### Limitations

**Out-of-tree modules**: If `go.work` references modules outside the workspace root (e.g., `use ../sibling`), Gorgon will reject files from those modules with a clear error. This is an uncommon pattern and can be addressed if needed.

## Organization-Level Policy (Enterprise Governance)

Large organizations need top-down policy enforcement that teams cannot weaken. The `gorgon-org.yml` file defines hard minimums and required settings that apply across all projects.

### How It Works

1. **Discovery**: Gorgon searches for `gorgon-org.yml` by walking up from the project root, or via `GORGON_ORG_POLICY` env var
2. **Enforcement**: Policy constraints are applied **after** all sub-config resolution
3. **Non-overrideable**: Teams cannot opt out or weaken org policy settings

### Example Org Policy

```yaml
# gorgon-org.yml - placed at org root or set via GORGON_ORG_POLICY env var

# Minimum score any package must achieve
threshold_floor: 65.0

# These operators run everywhere, regardless of team config
required_operators:
  - condition_negation
  - arithmetic_flip
  - error_handling

# These operators are prohibited org-wide
forbidden_operators:
  - empty_body

# Teams cannot change these settings
locked_settings:
  - skip_func      # teams cannot exempt functions
  - exclude        # teams cannot exclude files

# Generated code skipped everywhere
forced_skip_paths:
  - "*.pb.go"
  - "mock_*.go"

# All CI machines must use at least 4 cores
min_concurrent: 4

# Cache must always be on
require_cache: true
```

### Policy Fields

| Field | Description |
|-------|-------------|
| `threshold_floor` | Minimum mutation score. Sub-configs setting lower thresholds are raised to this value |
| `required_operators` | Operators that must run everywhere. Injected into all configs |
| `forbidden_operators` | Operators that are never allowed. Removed from all configs |
| `locked_settings` | Settings teams cannot override: `skip`, `skip_func`, `exclude`, `include`, `tests`, `cache`, `concurrent`, `operators`, `threshold` |
| `forced_skip_paths` | Paths always skipped (e.g., generated code) |
| `forced_exclude_patterns` | Glob patterns always excluded |
| `min_concurrent` | Minimum concurrency level |
| `require_cache` | Force cache to be enabled |

### Discovery Locations

Gorgon searches for `gorgon-org.yml` in this order:

1. **`GORGON_ORG_POLICY` env var** - Explicit path (highest priority)
2. **Walk up from project root** - Searches parent directories
3. **`$XDG_CONFIG_HOME/gorgon/gorgon-org.yml`** - User/org config directory

### Violation Reporting

When org policy constraints are applied, violations are logged:

```
Org policy applied 3 constraint(s):
  org policy: threshold was "40.00", enforced to "65.00" (below org threshold_floor)
  org policy: operators was "boundary_value", enforced to "condition_negation (injected)" (required by org policy)
  org policy: cache was "false", enforced to "true" (require_cache set in org policy)
```

Control violation behavior in your team config:

```yaml
# gorgon.yml
violation_mode: fail    # fail (default), warn, or silent
```

The org policy can lock this setting to prevent teams from silencing violations:

```yaml
# gorgon-org.yml
locked_settings:
  - violation_mode
```

### Team Config Options

Teams can configure how they work **within** policy constraints:

```yaml
# gorgon.yml

# How sub-configs inherit from parents
sub_config_mode: merge  # merge (default), replace, or isolate

# Propagate root threshold to sub-configs without their own
threshold_inherit: true

# How to handle policy violations
violation_mode: fail  # fail (default), warn, or silent
```

### Benefits

**Compliance**: Security-critical operators can be mandated org-wide  
**Prevent gaming**: Teams cannot set `threshold: 0` to bypass quality gates  
**Consistent standards**: Generated code handling, concurrency, caching enforced uniformly  
**Separation of concerns**: Platform teams own policy, app teams own their configs  
**Transparent**: Violations are logged so teams understand what was enforced  

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

## Go Version Configuration

Gorgon automatically detects the Go version from your project's `go.mod` file and uses it when generating temporary test modules. This ensures compatibility as Go evolves.

### Auto-Detection

By default, Gorgon reads the `go X.Y` directive from your project's `go.mod`:

```go
// go.mod
module myproject

go 1.25  // ← Gorgon uses this version
```

If detection fails, it falls back to `1.25`.

### Manual Override

Override the detected version in your config file:

```yaml
# gorgon.yml
go_version: "1.26"  # Force specific Go version
```

**Use cases**:
- Testing compatibility with a newer Go version
- Working in environments where go.mod detection fails
- Standardizing across multiple projects

**Note**: The version must be in `X.Y` format (e.g., `1.25`, `1.26`).

## Memory Optimization

For extremely large files with many mutants (>500), Gorgon can split them into separate compilation units to prevent out-of-memory errors during compilation.

### Chunking (Default: Enabled)

```yaml
# gorgon.yml
chunk_large_files: true  # Split files with >500 mutants (default)
```

When enabled, files with more than 500 mutants are automatically split into chunks:
- Each chunk compiles independently with ≤500 mutants
- Reduces peak memory usage during compilation
- Prevents compiler OOM kills on extremely large files

**Example**: A file with 1200 mutants splits into:
- Chunk 1: mutants 1-500 → `internal/cli/`
- Chunk 2: mutants 501-1000 → `internal/cli_chunk2/`
- Chunk 3: mutants 1001-1200 → `internal/cli_chunk3/`

### Disable Chunking

```yaml
chunk_large_files: false  # Compile all mutants together
```

Disable if:
- You have sufficient RAM (32GB+)
- No single file has >500 mutants
- You want slightly faster execution

**Trade-off**: Faster execution but may cause OOM on extremely large files.

**Note**: The preflight optimizations handle most cases efficiently. Chunking is only a safety net for truly massive files.

## Output Files

Write the report to a file by listing it in `outputs:`:

```yaml
outputs:
  - textfile:report.txt
```

This writes the full report (mutation score, top killers, killed/survived mutants) to `report.txt` while still printing to stdout.

### Debug Files

Run with `-debug` to also write detailed debug information alongside the textfile output. For `outputs: [textfile:report.txt]`, this produces:

- `report.txt` — the standard report (stats, killed mutants, survived mutants)
- `report.debug.txt` — detailed debug info (preflight rejections, error summaries, per-mutant compile errors)

If no textfile output is configured, debug data is written to `gorgon-debug.txt` in the current directory.

### Supported Formats

`textfile`, `html`, `junit`, `sarif`, `json`. All can appear together in a single `outputs:` list.

## CPU Profiling

Configure `cpu_profile` in `gorgon.yml` to write a CPU profile. Analyze it with:

```
go tool pprof -http=:8080 file.out
```

```yaml
cpu_profile: "gorgon.cpuprofile"   # explicit path
# or
cpu_profile: "true"                # writes to ./gorgon.cpuprofile
```

For heap profiles, use `-mem-profile=profiles/` (the only profiling flag exposed on the CLI) or `mem_profile: profiles` in the config — Gorgon writes a numbered series of pprof heap dumps into that directory.
