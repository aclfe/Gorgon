package loop_body_removal

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestLoopBodyRemoval_Name(t *testing.T) {
	op := LoopBodyRemoval{}
	if op.Name() != "loop_body_removal" {
		t.Errorf("expected name 'loop_body_removal', got '%s'", op.Name())
	}
}

func TestLoopBodyRemoval_CanApply(t *testing.T) {
	op := LoopBodyRemoval{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestLoopBodyRemoval_Registration(t *testing.T) {
	_, ok := mutator.Get("loop_body_removal")
	if !ok {
		t.Error("expected loop_body_removal to be registered")
	}
}

var _ mutator.Operator = LoopBodyRemoval{}
var _ mutator.ContextualOperator = LoopBodyRemoval{}
