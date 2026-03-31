package constant_replacement

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestConstantReplacement_Name(t *testing.T) {
	op := ConstantReplacement{}
	if op.Name() != "constant_replacement" {
		t.Errorf("expected name 'constant_replacement', got '%s'", op.Name())
	}
}

func TestConstantReplacement_Registration(t *testing.T) {
	_, ok := mutator.Get("constant_replacement")
	if !ok {
		t.Error("expected constant_replacement to be registered")
	}
}

var _ mutator.Operator = ConstantReplacement{}
