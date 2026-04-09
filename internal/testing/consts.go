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

// Minimum baseline duration for timeout calculations.
const minBaselineDuration = 100

// File permissions for generated files.
const filePermissions = 0o600
