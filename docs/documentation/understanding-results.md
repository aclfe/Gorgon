# Understanding Results

Every generated mutant is reported exactly once, with one of the statuses
below. Each status has a single meaning.

## Statuses

| Status | Meaning | In the score |
|---|---|---|
| `killed` | At least one test failed, or the test binary crashed, with the mutation active. The same tests passed without any mutation. `killed_by` names the first failing test. | detected |
| `timeout` | The tests did not finish within the mutant's timeout. | detected |
| `survived` | The tests ran and passed with the mutation active. | not detected |
| `untested` | No test exercised the mutant: the package has no test files, the configured test filter matched no test, or the package is outside the configured `tests:`. | not detected |
| `compile_error` | The mutated program does not compile. The mutant was never run. | excluded |
| `invalid` | Not a mutation that can be run at this site: the operator produced no change, or the mutation could not be injected. The reason is in `reason`. | excluded |
| `error` | The mutant could not be evaluated: the package's tests fail even without any mutation (baseline failure), its test binary cannot be built in the workspace, or the run was cancelled. The reason is in `reason`. | excluded |

## Mutation score

```
score = (killed + timeout) / (killed + timeout + survived + untested) * 100
```

`compile_error`, `invalid` and `error` mutants are excluded: the tests never
judged them.

## How a verdict is reached

1. **Discovery.** A node is a mutation site only when an operator applies to
   it, the operator is enabled for the file (operators, sub-configs,
   `dir_rules`), and it is not suppressed (`//gorgon:ignore`, config
   `suppress:`, sub-config `suppress:`). Operators whose mutation would not
   change the program, and operators that would produce the same program as
   another operator at the same place, are not generated.
2. **Injection.** All mutants are compiled into one copy of the module, each
   guarded by `activeMutantID == N`. A mutant whose guard cannot be placed is
   `invalid` and never runs, because running it would run the original code.
3. **Validation.** Mutants are type-checked; those that do not compile are
   `compile_error`. The test binaries are then built and any remaining build
   error is attributed to the mutant whose guarded code contains it (or found
   by bisection).
4. **Baseline.** Each test binary first runs with no mutation active. If that
   run fails, the package's mutants are `error` ("baseline failed") instead of
   being blamed for failures they did not cause. The baseline duration sets
   the per-mutant timeout (3x baseline + 2s).
5. **Execution.** Each mutant runs the binary with `GORGON_MUTANT_ID=N`. The
   exit status decides: non-zero is `killed`, zero is `survived`; the
   testing package's own timeout panic or the hard deadline is `timeout`.
6. **External suites.** Mutants not yet detected also run against external
   suite binaries, each with its own baseline. The strongest outcome wins
   (killed > timeout > survived > error > untested).

The workspace is a full copy of the module (including `testdata/`, embedded
files and `vendor/`), so tests see the same files they see in your tree.

## Caching

With `cache: true`, only `killed`, `survived` and `timeout` verdicts are
cached. A cached verdict is reused only when the mutated package, its tests,
its testdata, every in-module package it depends on, `go.mod`/`go.sum`, the
build tags and the test configuration are unchanged.
