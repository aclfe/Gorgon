package testing

// Go version used in generated go.mod files.
const goVersion = "1.25"

// Default module name used in standalone benchmarks.
const defaultModuleName = "gorgon-standalone"

// Default module name used in bench configurations.
const benchModuleName = "gorgon-bench"

// Timeout multiplier for mutant test execution relative to baseline.
const timeoutMultiplier = 3.0

// Maximum timeout for mutant test execution.
const maxTimeout = 30

// Minimum baseline duration for timeout calculations (ms).
const minBaselineDuration = 100

// Maximum baseline duration to prevent inflated per-mutant timeouts (5s).
const maxBaselineCap = 5 * 1e9 // 5 seconds in nanoseconds

// Minimum per-mutant timeout (500ms) — prevents fast tests from being killed.
const minMutantTimeout = 500 * 1e6 // 500ms in nanoseconds

// Extra margin added to hard timeout beyond -test.timeout flag.
const hardTimeoutMargin = 2e9 // 2 seconds in nanoseconds

// File permissions for generated files.
const filePermissions = 0o600
