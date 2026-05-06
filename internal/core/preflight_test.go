package testing

import (
	"go/token"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
)

func TestLevel1_NilNodeMarkedInvalid(t *testing.T) {
	mutants := []Mutant{
		{ID: 1, Site: engine.Site{Node: nil, File: &token.File{}}},
	}
	valid, invalid := quickStaticFilter(mutants)

	if len(valid) != 0 {
		t.Fatalf("expected 0 valid, got %d", len(valid))
	}
	if len(invalid) != 1 || invalid[0].ErrorReason != "nil node" {
		t.Fatalf("expected 1 invalid with 'nil node', got %+v", invalid)
	}
}
