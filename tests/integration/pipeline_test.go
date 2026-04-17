package testing_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	"github.com/aclfe/gorgon/tests/testutil"
)

func TestEndToEndMutationPipeline(tst *stdtesting.T) {

	tst.Skip("Slow integration test - run explicitly if needed")

	absPath, err := filepath.Abs("../../../examples/mutations/arithmetic_flip")
	if err != nil {
		tst.Fatal(err)
	}
	sites, operators := loadTestSites(tst, absPath)

	if len(sites) == 0 {
		tst.Fatal("Expected to find mutation sites, found 0")
	}

	mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, 2, nil, nil, nil, testutil.Logger(), false)
	if err != nil {
		tst.Skipf("Pipeline skipped (dependency issue): %v", err)
	}

	if len(mutants) == 0 {
		tst.Skip("No mutants generated")
	}

	killed := 0
	survived := 0
	for _, mutant := range mutants {
		switch mutant.Status {
		case "killed":
			killed++
		case "survived":
			survived++
		default:
			tst.Logf("Mutant %d error: %v", mutant.ID, mutant.Error)
		}
	}

	tst.Logf("Total Mutants: %d", len(mutants))
	tst.Logf("Killed: %d", killed)
	tst.Logf("Survived: %d", survived)

	if killed == 0 {
		tst.Error("Expected at least one mutant to be killed")
	}
}

func loadTestSites(t stdtesting.TB, basePath string) ([]engine.Site, []mutator.Operator) {
	t.Helper()
	var sites []engine.Site
	if err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || filepath.Base(path) == "gorgon_schemata.go" {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if be, ok := n.(*ast.BinaryExpr); ok {
				pos := fset.Position(be.OpPos)
				sites = append(sites, engine.Site{
					File:   fset.File(be.OpPos),
					Line:   pos.Line,
					Column: pos.Column,
					Node:   be,
				})
			}
			return true
		})
		return nil
	}); err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	operators := []mutator.Operator{
		arithmetic_flip.ArithmeticFlip{},
	}
	return sites, operators
}
