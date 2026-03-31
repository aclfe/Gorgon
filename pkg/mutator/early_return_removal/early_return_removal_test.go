package early_return_removal

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestEarlyReturnRemoval_Name(t *testing.T) {
	op := EarlyReturnRemoval{}
	if op.Name() != "early_return_removal" {
		t.Errorf("expected name 'early_return_removal', got '%s'", op.Name())
	}
}

func TestEarlyReturnRemoval_CanApply(t *testing.T) {
	op := EarlyReturnRemoval{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestEarlyReturnRemoval_Registration(t *testing.T) {
	_, ok := mutator.Get("early_return_removal")
	if !ok {
		t.Error("expected early_return_removal to be registered")
	}
}

var _ mutator.Operator = EarlyReturnRemoval{}
var _ mutator.ContextualOperator = EarlyReturnRemoval{}
