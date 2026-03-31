package negate_condition

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestNegateCondition_Name(t *testing.T) {
	op := NegateCondition{}
	if op.Name() != "negate_condition" {
		t.Errorf("expected name 'negate_condition', got '%s'", op.Name())
	}
}

func TestNegateCondition_CanApply(t *testing.T) {
	op := NegateCondition{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestNegateCondition_Registration(t *testing.T) {
	_, ok := mutator.Get("negate_condition")
	if !ok {
		t.Error("expected negate_condition to be registered")
	}
}

var _ mutator.Operator = NegateCondition{}
var _ mutator.ContextualOperator = NegateCondition{}
