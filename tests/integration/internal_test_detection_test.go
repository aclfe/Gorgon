//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gorgontesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
)

func TestInternalTestDetection(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	projectRoot := filepath.Join(cwd, "..", "..")
	cliDir := filepath.Join(projectRoot, "internal", "cli")

	if _, err := os.Stat(cliDir); os.IsNotExist(err) {
		t.Skipf("internal/cli directory not found at %s", cliDir)
	}

	eng := engine.NewEngine(false)
	ops := []mutator.Operator{&arithmetic_flip.ArithmeticFlip{}}
	eng.SetOperators(ops)
	if err := eng.Traverse(cliDir, nil); err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}

	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("No mutation sites found in internal/cli")
	}

	t.Logf("Found %d mutation sites in internal/cli", len(sites))

	cfg := config.Default()
	cfg.ChunkLargeFiles = true

	mutants, err := gorgontesting.GenerateAndRunSchemata(
		context.Background(),
		sites,
		ops,
		ops,
		projectRoot,
		projectRoot,
		nil,
		nil,
		2,
		nil,
		nil,
		nil,
		logger.New(false),
		false,
		true,
		config.ExternalSuitesConfig{},
		cfg,
	)

	if err != nil {
		t.Fatalf("GenerateAndRunSchemata failed: %v", err)
	}

	var killed, survived, untested, errors int
	for _, m := range mutants {
		switch m.Status {
		case "killed":
			killed++
		case "survived":
			survived++
		case "untested":
			untested++
		case "error":
			errors++
		}
	}

	t.Logf("Results: %d killed, %d survived, %d untested, %d errors (total: %d)", killed, survived, untested, errors, len(mutants))

	// The test file exists in internal/cli, so we expect at least one result
	if untested == len(mutants) {
		t.Errorf("Expected at least one killed/survived/error mutant, got all untested (%d)", untested)
	}

	if untested > 0 && untested < len(mutants) {
		t.Logf("Some mutants were tested (%d untested out of %d)", untested, len(mutants))
	}

	foundKilledByTest := false
	for _, m := range mutants {
		if m.Status == "killed" && m.KilledBy != "" {
			t.Logf("Mutant #%d killed by: %s", m.ID, m.KilledBy)
			foundKilledByTest = true
			break // Just log one example
		}
	}

	if killed > 0 && !foundKilledByTest {
		t.Error("Mutants were killed but no test attribution found")
	}
}

func TestInternalTestDetection_NoTests(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	projectRoot := filepath.Join(cwd, "..", "..")
	notestsDir := filepath.Join(projectRoot, "pkg", "testdata", "notests")

	if _, err := os.Stat(notestsDir); os.IsNotExist(err) {
		t.Skipf("pkg/testdata/notests directory not found at %s", notestsDir)
	}

	eng := engine.NewEngine(false)
	ops := []mutator.Operator{&arithmetic_flip.ArithmeticFlip{}}
	eng.SetOperators(ops)
	if err := eng.Traverse(notestsDir, nil); err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}

	sites := eng.Sites()
	if len(sites) == 0 {
		t.Skip("No mutation sites found in pkg/testdata/notests")
	}

	t.Logf("Found %d mutation sites in pkg/testdata/notests", len(sites))

	cfg := config.Default()
	cfg.ChunkLargeFiles = true

	mutants, err := gorgontesting.GenerateAndRunSchemata(
		context.Background(),
		sites,
		ops,
		ops,
		projectRoot,
		projectRoot,
		nil,
		nil,
		2,
		nil,
		nil,
		nil,
		logger.New(false),
		false,
		true,
		config.ExternalSuitesConfig{},
		cfg,
	)

	if err != nil {
		t.Fatalf("GenerateAndRunSchemata failed: %v", err)
	}

	var killed, survived, untested, errors int
	for _, m := range mutants {
		switch m.Status {
		case "killed":
			killed++
		case "survived":
			survived++
		case "untested":
			untested++
		case "error":
			errors++
		}
	}

	t.Logf("Results: %d killed, %d survived, %d untested, %d errors (total: %d)", killed, survived, untested, errors, len(mutants))

	// When there are no test files, all mutants should be untested
	if untested == 0 && errors == 0 {
		t.Error("Expected untested mutants when no test file exists, got 0")
	}

	if killed > 0 {
		t.Errorf("Expected 0 killed mutants without tests, got %d", killed)
	}
}
