//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gcache "github.com/aclfe/gorgon/internal/cache"
	gcore "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/condition_negation"
	"github.com/aclfe/gorgon/tests/testutil"
)

func countTempDir() int {
	dirs, _ := os.ReadDir(os.TempDir())
	return len(dirs)
}

func TestGenerateAndRunSchemata_ZeroMutantsEarlyReturn(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	dir := t.TempDir()
	testutil.CreateFixtureModule(t, dir, map[string]string{
		"go.mod": `module testproject

go 1.25
`,
		"main.go": `package main

func Add(a, b int) int {
	return a + b
}
`,
		"main_test.go": `package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("failed")
	}
}
`,
	})

	log := logger.New(false)
	cache := gcache.New()

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		[]engine.Site{},
		nil, nil, dir, dir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		config.ExternalSuitesConfig{}, nil,
	)

	if err != nil {
		t.Fatalf("expected nil error for zero mutants, got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for zero mutants, got %d mutants", len(result))
	}
}

func TestGenerateAndRunSchemata_AllCachedNoExternal(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "progress_bar")
	log := logger.New(false)
	cache := gcache.New()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	result1, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("first run unexpected error: %v", err)
	}
	if result1 == nil {
		t.Fatal("first run returned nil result")
	}

	result2, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("second run unexpected error: %v", err)
	}
	if result2 == nil {
		t.Fatal("second run returned nil result")
	}

	if len(result1) != len(result2) {
		t.Fatalf("result lengths differ: run1=%d, run2=%d", len(result1), len(result2))
	}

	for i := 0; i < len(result1); i++ {
		if result1[i].Status != result2[i].Status {
			t.Fatalf("mutant %d status differs: run1=%s, run2=%s", i, result1[i].Status, result2[i].Status)
		}
	}
}

func TestGenerateAndRunSchemata_ModuleLayouts(t *testing.T) {
	tests := []struct {
		desc     string
		fixture  string
		operator string
	}{
		{
			desc:     "arithmetic_flip",
			fixture:  "arithmetic_flip",
			operator: "arithmetic_flip",
		},
		{
			desc:     "boundary_value",
			fixture:  "boundary_value",
			operator: "boundary_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			testutil.SetupIsolatedEnv(t)

			fixtureDir := filepath.Join("testdata", tt.fixture)
			absPath, err := filepath.Abs(fixtureDir)
			if err != nil {
				t.Fatal(err)
			}

			log := logger.New(false)
			cache := gcache.New()

			eng := engine.NewEngine(false)
			op, ok := mutator.Get(tt.operator)
			if !ok {
				t.Fatalf("%s operator not found", tt.operator)
			}
			eng.SetOperators([]mutator.Operator{op})
			if err := eng.Traverse(absPath, nil); err != nil {
				t.Fatalf("traverse failed: %v", err)
			}
			sites := eng.Sites()
			if len(sites) == 0 {
				t.Fatalf("no mutation sites found in fixture with %s operator", tt.operator)
			}

			result, err := gcore.TestGenerateAndRunSchemata(
				context.Background(),
				sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
				nil, nil, 1, cache, nil, nil, log, false, true,
				config.ExternalSuitesConfig{}, nil,
			)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatalf("expected non-nil result with %d sites", len(sites))
			}

			if len(result) == 0 {
				t.Fatalf("expected non-zero result length with %d sites", len(sites))
			}
		})
	}
}

func TestGenerateAndRunSchemata_ProgressBarLifecycle(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "progress_bar")
	log := logger.New(false)
	cache := gcache.New()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, true, true,
		config.ExternalSuitesConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("returned nil result")
	}
}

func TestGenerateAndRunSchemata_ExternalSuites(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "external_suites")
	log := logger.New(false)
	cache := gcache.New()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	cfg := config.ExternalSuitesConfig{
		Enabled: true,
		RunMode: "after_unit",
		Suites: []config.ExternalSuite{
			{
				Name:  "external-tests",
				Paths: []string{fixtureDir},
			},
		},
	}

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		cfg, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("returned nil result")
	}
}

func TestRunStandalone_TempLeakCheck(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	beforeCount := countTempDir()

	fixtureDir := filepath.Join("testdata", "arithmetic_flip")
	absPath, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	log := logger.New(false)
	cache := gcache.New()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(absPath, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	_, err = gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		config.ExternalSuitesConfig{}, nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	afterCount := countTempDir()

	if afterCount != beforeCount {
		t.Fatalf("temp dir count increased: before=%d, after=%d", beforeCount, afterCount)
	}
}

func TestRunStandalone_ContextCancellation(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "arithmetic_flip")
	absPath, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	log := logger.New(false)
	cache := gcache.New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(absPath, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	_, err = gcore.TestGenerateAndRunSchemata(
		ctx,
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		config.ExternalSuitesConfig{}, nil,
	)

	if err == nil {
		t.Fatalf("expected error from cancelled context, got nil")
	}
}

func TestGenerateAndRunSchemata_Determinism(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "arithmetic_flip")
	absPath, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	var results [2][]gcore.Mutant
	for i := 0; i < 2; i++ {
		log := logger.New(false)
		cache := gcache.New()

		eng := engine.NewEngine(false)
		op, ok := mutator.Get("arithmetic_flip")
		if !ok {
			t.Fatal("arithmetic_flip operator not found")
		}
		eng.SetOperators([]mutator.Operator{op})
		if err := eng.Traverse(absPath, nil); err != nil {
			t.Fatalf("run %d traverse failed: %v", i+1, err)
		}
		sites := eng.Sites()
		if len(sites) == 0 {
			t.Fatalf("run %d no mutation sites found in fixture", i+1)
		}

		result, err := gcore.TestGenerateAndRunSchemata(
			context.Background(),
			sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
			nil, nil, 1, cache, nil, nil, log, false, true,
			config.ExternalSuitesConfig{}, nil,
		)

		if err != nil {
			t.Fatalf("run %d unexpected error: %v", i+1, err)
		}

		results[i] = result
	}

	if len(results[0]) != len(results[1]) {
		t.Fatalf("result lengths differ: run1=%d, run2=%d", len(results[0]), len(results[1]))
	}

	for i := 0; i < len(results[0]); i++ {
		if results[0][i].Status != results[1][i].Status {
			t.Fatalf("mutant %d status differs: run1=%s, run2=%s", i, results[0][i].Status, results[1][i].Status)
		}
	}
}

func TestGenerateAndRunSchemata_AccountingInvariant(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "arithmetic_flip")
	absPath, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	log := logger.New(false)
	cache := gcache.New()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(absPath, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	result, err := gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
		nil, nil, 1, cache, nil, nil, log, false, true,
		config.ExternalSuitesConfig{}, nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatalf("expected non-nil result with %d sites", len(sites))
	}

	validStatuses := map[string]bool{
		"":              true,
		"killed":        true,
		"survived":      true,
		"error":         true,
		"timeout":       true,
		"untested":      true,
		"compile_error": true,
	}

	for i, m := range result {
		if !validStatuses[m.Status] {
			t.Fatalf("mutant %d has unknown status: %s", i, m.Status)
		}
	}
}

func TestFilterMutantsWithoutTests_Packages(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")

	content := `module testproject

go 1.25
`
	if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	mainGo := `package main

func Add(a, b int) int { return a + b }
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	mainTest := `package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("failed")
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(mainTest), 0644); err != nil {
		t.Fatalf("failed to create main_test.go: %v", err)
	}

	absBase, _ := filepath.Abs(dir)
	coveredPackages := gcore.TestCollectPackagesWithTests(absBase)

	if !coveredPackages["."] {
		t.Fatalf("main package not covered by tests")
	}
}

func TestExtractMutantIDsFromBuildErrors_Window(t *testing.T) {
	dir := t.TempDir()

	testFile := filepath.Join(dir, "test.go")
	testContent := `package main

func Add(a, b int) int {
	return a + b // activeMutantID == 123
}

func main() {
	_ = UndefinedFunction()
}
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	goMod := `module testproject

go 1.25
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	cmd := exec.CommandContext(context.Background(), "go", "build", "./...")
	cmd.Dir = dir
	output, _ := cmd.CombinedOutput()

	t.Logf("Build output: %s", string(output))

	ids := gcore.TestExtractMutantIDsFromBuildErrors(dir, string(output))

	t.Logf("Found IDs: %v", ids)

	found := false
	for _, id := range ids {
		if id == 123 {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("mutant ID 123 not found in build errors: %v", ids)
	}
}

func TestMakeSelfContained_Idempotency(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")

	content1 := `module testproject

go 1.25
`
	if err := os.WriteFile(goModPath, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	err := gcore.MakeSelfContained(dir)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	result1, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	err = gcore.MakeSelfContained(dir)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	result2, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	if string(result1) != string(result2) {
		t.Fatalf("not idempotent:\nfirst:\n%s\nsecond:\n%s", string(result1), string(result2))
	}

	lines1 := strings.Count(string(result1), "\n")
	lines2 := strings.Count(string(result2), "\n")

	if lines1 != lines2 {
		t.Fatalf("line count changed: first=%d, second=%d", lines1, lines2)
	}
}

func TestMakeSelfContained_Unreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denial as root")
	}

	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")

	content := `module testproject

go 1.25
`
	if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	if err := os.Chmod(goModPath, 0o000); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	err := gcore.MakeSelfContained(dir)
	if err == nil {
		t.Fatalf("expected error for unreadable go.mod")
	}

	_ = os.Chmod(goModPath, 0o644)
}

func TestRunExternalPhase_RunModes(t *testing.T) {
	dir := t.TempDir()

	goMod := `module testproject

go 1.25
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	mainGo := `package main

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	mainTest := `package main

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("expected 5, got %d", result)
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(mainTest), 0644); err != nil {
		t.Fatalf("failed to create main_test.go: %v", err)
	}

	tests := []struct {
		name    string
		runMode string
	}{
		{"only mode", "only"},
		{"after_unit mode", "after_unit"},
		{"alongside mode", "alongside"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupIsolatedEnv(t)

			ws, err := gcore.NewModuleWorkspace()
			if err != nil {
				t.Fatalf("failed to create workspace: %v", err)
			}
			defer ws.Cleanup()

			if err := ws.Setup(dir, nil); err != nil {
				t.Fatalf("failed to setup workspace: %v", err)
			}

			cfg := config.ExternalSuitesConfig{
				Enabled: true,
				RunMode: tt.runMode,
				Suites: []config.ExternalSuite{
					{
						Name:  "test-suite",
						Paths: []string{dir},
					},
				},
			}

			log := logger.New(false)

			mutants := []gcore.Mutant{
				{ID: 1, Status: "survived"},
				{ID: 2, Status: "killed"},
			}

			err = gcore.TestRunExternalPhase(context.Background(), ws, mutants, cfg, 1, log)

			if err != nil {
				t.Fatalf("runExternalPhase returned error: %v", err)
			}
		})
	}
}

func TestRunStandalone_ConcurrentLimit(t *testing.T) {
	testutil.SetupIsolatedEnv(t)

	fixtureDir := filepath.Join("testdata", "concurrent_limit")
	log := logger.New(false)
	cache := gcache.New()

	eng := engine.NewEngine(false)
	op, ok := mutator.Get("arithmetic_flip")
	if !ok {
		t.Fatal("arithmetic_flip operator not found")
	}
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(fixtureDir, nil); err != nil {
		t.Fatalf("traverse failed: %v", err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatal("no mutation sites found in fixture")
	}

	concurrentLimits := []int{1, 2, 4}

	for _, limit := range concurrentLimits {
		t.Run(fmt.Sprintf("concurrent=%d", limit), func(t *testing.T) {
			result, err := gcore.TestGenerateAndRunSchemata(
				context.Background(),
				sites, []mutator.Operator{op}, []mutator.Operator{op}, fixtureDir, fixtureDir,
				nil, nil, limit, cache, nil, nil, log, false, true,
				config.ExternalSuitesConfig{}, nil,
			)
			if err != nil {
				t.Fatalf("concurrent=%d unexpected error: %v", limit, err)
			}
			if result == nil {
				t.Fatalf("concurrent=%d returned nil result", limit)
			}
		})
	}
}
