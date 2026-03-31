package empty_body

import (
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestEmptyBody_Name(t *testing.T) {
	op := EmptyBody{}
	if op.Name() != "empty_body" {
		t.Errorf("expected name 'empty_body', got '%s'", op.Name())
	}
}

func TestEmptyBody_CanApply(t *testing.T) {
	op := EmptyBody{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestEmptyBody_Registration(t *testing.T) {
	_, ok := mutator.Get("empty_body")
	if !ok {
		t.Error("expected empty_body to be registered")
	}
}

var _ mutator.Operator = EmptyBody{}
var _ mutator.ContextualOperator = EmptyBody{}
