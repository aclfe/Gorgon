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
| `-exclude` | `""` | Comma-separated glob patterns for files to exclude |
| `-include` | `""` | Comma-separated glob patterns for files to include |
| `-skip` | `""` | Comma-separated relative file paths to skip entirely |
| `-skip-func` | `""` | Comma-separated file:function pairs to skip (e.g. foo/bar.go:MyFunc) |
| `-tests` | `""` | Comma-separated relative paths to test files/folders to run |

## Config

Use `-config` to load a YAML file. All flags must be omitted when using `-config`.

```yaml
operators:
  - all
concurrent: all
threshold: 80
cache: true
dry_run: false
exclude:
  - "*_test.go"
include: []
skip:
  - vendor/
skip_func:
  - foo/bar.go:MyFunc
tests: []
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

Relative paths are resolved from the target directory.

### Auto Syncing

When running with `-config`, inline `//gorgon:ignore` comments are automatically added to the config file's `suppress:` section. Each comment becomes a YAML entry with the relative file path, line number, and suppressed operators:

```yaml
suppress:
  - location: pkg/file.go:12
    operators:
      - panic_removal
```

Existing config suppressions are preserved and merged with inline comments.

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
