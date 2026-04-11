package defer_removal

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestDeferRemoval_Name(t *testing.T) {
	op := DeferRemoval{}
	if op.Name() != "defer_removal" {
		t.Errorf("expected name 'defer_removal', got '%s'", op.Name())
	}
}

func TestDeferRemoval_CanApply(t *testing.T) {
	op := DeferRemoval{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestDeferRemoval_Registration(t *testing.T) {
	_, ok := mutator.Get("defer_removal")
	if !ok {
		t.Error("expected defer_removal to be registered")
	}
}

var _ mutator.Operator = DeferRemoval{}
var _ mutator.ContextualOperator = DeferRemoval{}
