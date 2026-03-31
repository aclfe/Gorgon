package variable_replacement

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestVariableReplacement_Name(t *testing.T) {
	op := VariableReplacement{}
	if op.Name() != "variable_replacement" {
		t.Errorf("expected name 'variable_replacement', got '%s'", op.Name())
	}
}

func TestVariableReplacement_CanApply(t *testing.T) {
	op := VariableReplacement{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestVariableReplacement_Registration(t *testing.T) {
	_, ok := mutator.Get("variable_replacement")
	if !ok {
		t.Error("expected variable_replacement to be registered")
	}
}

var _ mutator.Operator = VariableReplacement{}
var _ mutator.ContextualOperator = VariableReplacement{}
