//go:build integration
// +build integration

// Package extsuite is a small external test suite used by the e2e external
// suite tests: it exercises internal/core preflight from outside the package,
// fast enough to run once per mutant.
package extsuite

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/mutator"

	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
)

func TestPreflightRejectsNilNode(t *testing.T) {
	mutants := []coretesting.Mutant{{ID: 1, Site: engine.Site{File: &token.File{}}}}
	valid, rejected := coretesting.RunPreflight(mutants, logger.New(false))
	if len(valid) != 0 {
		t.Fatalf("nil-node mutant must not pass preflight, got %d valid", len(valid))
	}
	if len(rejected) != 1 || rejected[0].Status != coretesting.StatusInvalid || rejected[0].ErrorReason != "nil node" {
		t.Fatalf("want one invalid result with reason 'nil node', got %+v", rejected)
	}
}

func TestPreflightKeepsValidMutant(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.22\n",
		"a.go":   "package m\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip not registered")
	}
	ops := []mutator.Operator{op}
	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	eng.SetProjectRoot(dir)
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatal(err)
	}
	mutants := coretesting.GenerateMutants(eng.Sites(), ops, ops, dir, nil, nil, logger.New(false))
	if len(mutants) != 1 {
		t.Fatalf("want 1 mutant, got %d", len(mutants))
	}
	valid, rejected := coretesting.RunPreflight(mutants, logger.New(false))
	if len(valid) != 1 || len(rejected) != 0 {
		t.Fatalf("a valid mutant must pass preflight: %d valid, rejected %+v", len(valid), rejected)
	}
	if valid[0].ID != mutants[0].ID {
		t.Fatalf("preflight returned a different mutant: %d", valid[0].ID)
	}
}
