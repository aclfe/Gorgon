## Gorgon v0.5.2

Go mutation testing tool.

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
| `-debug` | `false` | Show detailed debug output during execution |
| `-show-killed` | `false` | Show killed mutants with test attribution |
| `-format` | `textfile` | Output format for report file |
| `-output` | `""` | Write report to file (e.g. `report.txt`) |
| `-debug-files` | `false` | Also write debug info to `{output}.debug.txt` |
| `-cpu-profile` | `""` | Write CPU profile to file (analyzable with `go tool pprof`) |

## Config

Use `-config` to load a YAML file. All flags must be omitted when using `-config`.

```yaml
operators:
  - all
concurrent: all
threshold: 80
cache: true
progbar: false
dry_run: false
show_killed: false
format: textfile
output: "report.txt"
debug_files: false
exclude:
  - "*_test.go"
include: []
skip:
  - vendor/
skip_func:
  - foo/bar.go:MyFunc
tests: []
debug: false
base: ""
cpu_profile: ""
```

```
gorgon -config=gorgon.yml ./path
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
- Schemata-based for fast testing
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

Currently only `textfile` format is supported.

## CPU Profiling

Use `-cpu-profile=file.out` or `cpu_profile: "file.out"` to write a CPU profile. Analyze it with:

```
go tool pprof -http=:8080 file.out
```

Use `cpu_profile: "true"` to write to `gorgon.cpuprofile` in the current directory.
