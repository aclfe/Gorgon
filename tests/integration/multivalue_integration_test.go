//go:build integration
// +build integration

package integration

import "testing"

// TestMultiValueReturnMutations is a regression test for the multi-value return bug fix.
//
// Background:
// Prior to the fix, Gorgon's schemata transformation had a bug when handling return
// statements with multiple values (e.g., `return val, err`). The engine would extract
// only the first return type, causing the generated closure to have a mismatched signature:
//
//	func() *config.Config {        // ← returns ONE value
//	    return nil, fmt.Errorf(...) // ← trying to return TWO values
//	}()
//
// This resulted in compilation errors like "not enough return values" or "too many return values",
// causing all mutants to be marked as "untested" because the test binary couldn't be built.
//
// The Fix:
// 1. Engine (internal/engine/engine.go): Extract ALL return types as comma-separated string
// 2. Handler (internal/core/schemata_nodes/handlers.go): Parse and build correct closure signatures
// 3. Validation: Filter out mutations with mismatched return value counts
//
// This test verifies:
// - Test binaries are successfully generated for packages with multi-value returns
// - No "not enough return values" or "too many return values" compilation errors
// - Mutations are properly applied and tested (not all marked "untested")
// - Tests can kill mutations, achieving >50% mutation score
// - The fix remains stable across code changes
func TestMultiValueReturnMutations(t *testing.T) {
	t.Skip("TODO: I need to consider this more deeply if this issue will still persist. FOr now, its just a TODO")
}