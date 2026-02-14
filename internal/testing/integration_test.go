package testing_test

import (
	"context"
	"path/filepath"
	stdtesting "testing"

	"github.com/aclfe/gorgon/internal/testing"
)

func TestEndToEndMutationPipeline(tst *stdtesting.T) {
	absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
	if err != nil {
		tst.Fatal(err)
	}
	sites, operators := loadTestSites(tst, absPath)

	if len(sites) == 0 {
		tst.Fatal("Expected to find mutation sites, found 0")
	}

	mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, 2)
	if err != nil {
		tst.Fatalf("Pipeline failed: %v", err)
	}

	if len(mutants) == 0 {
		tst.Fatal("No mutants generated")
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
