package loop_break_removal

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestLoopBreakRemoval_Name(t *testing.T) {
	op := LoopBreakRemoval{}
	if op.Name() != "loop_break_removal" {
		t.Errorf("expected name 'loop_break_removal', got '%s'", op.Name())
	}
}

func TestLoopBreakRemoval_CanApply(t *testing.T) {
	op := LoopBreakRemoval{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestLoopBreakRemoval_Registration(t *testing.T) {
	_, ok := mutator.Get("loop_break_removal")
	if !ok {
		t.Error("expected loop_break_removal to be registered")
	}
}

var _ mutator.Operator = LoopBreakRemoval{}
var _ mutator.ContextualOperator = LoopBreakRemoval{}
