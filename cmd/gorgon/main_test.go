package main

import (
	"os"
	"testing"
)

func TestMain(t *testing.T) {
	t.Parallel()
	// Test passes if main doesn't panic
	// Mock args to prevent os.Exit(1)
	os.Args = []string{"gorgon", "-print-ast", "../../test/testdata/astprint/print.go"}
	main()
}
