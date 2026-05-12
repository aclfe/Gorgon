//go:build integration
// +build integration

package integration

import "testing"

// TestWorkflow_PreflightCatchesBaselineErrors verifies preflight catches baseline errors
func TestWorkflow_PreflightCatchesBaselineErrors(t *testing.T) {
	t.Skip("TODO: Verify preflight catches pre-existing type errors")
}

func TestWorkflow_AllPreflightPhasesWork(t *testing.T) {
	t.Skip("TODO: We'll verify that all phases are able to filter mutations properly and not just be dummy.")
}
