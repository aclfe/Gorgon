package testing_test

import (
	"testing"

	"github.com/aclfe/gorgon/examples/mutations/arithmetic_flip"
)

// not an "integration test", I'm just checking external test suite functionality
func TestExample2(t *testing.T) {
	result := arithmetic_flip.Example2(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}
