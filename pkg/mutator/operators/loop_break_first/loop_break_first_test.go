package loop_break_first

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestLoopBreakFirst_Name(t *testing.T) {
	op := LoopBreakFirst{}
	if op.Name() != "loop_break_first" {
		t.Errorf("expected name 'loop_break_first', got '%s'", op.Name())
	}
}

func TestLoopBreakFirst_CanApply(t *testing.T) {
	op := LoopBreakFirst{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestLoopBreakFirst_Registration(t *testing.T) {
	_, ok := mutator.Get("loop_break_first")
	if !ok {
		t.Error("expected loop_break_first to be registered")
	}
}

var _ mutator.Operator = LoopBreakFirst{}
var _ mutator.ContextualOperator = LoopBreakFirst{}
