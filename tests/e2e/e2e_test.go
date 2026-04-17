package e2e_test

import (
	"testing"

	"github.com/aclfe/gorgon/examples/mutations/arithmetic_flip"
)

// not an "e2e" test, I'm just checking external test suite functionality
func TestE2EMultiply(t *testing.T) {
	result := arithmetic_flip.Multiply(3, 4)
	if result != 12 {
		t.Errorf("Expected 12, got %d", result)
	}
}
