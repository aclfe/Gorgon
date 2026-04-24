package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gcache "github.com/aclfe/gorgon/internal/cache"
	gcore "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// setupIsolatedEnv sets up environment isolation for tests
func SetupIsolatedEnv(t *testing.T) {
	t.Helper()
	gomodcache := t.TempDir()
	gocache := t.TempDir()

	t.Setenv("GOCACHE", gocache)
	t.Setenv("GOMODCACHE", gomodcache)
	t.Setenv("GOPATH", "")
	t.Setenv("GOFLAGS", "")

	t.Cleanup(func() {
		MakeWritable(gomodcache)
		MakeWritable(gocache)
	})
}

// MakeWritable recursively chmods everything under dir so os.RemoveAll can delete it
func MakeWritable(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			_ = os.Chmod(path, 0755)
		} else {
			_ = os.Chmod(path, 0644)
		}
		return nil
	})
}

// CreateFixtureModule creates a simple Go module fixture
func CreateFixtureModule(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", name, err)
		}
	}
}

// CreateFixtureWorkspace creates a Go workspace fixture
func CreateFixtureWorkspace(t *testing.T, dir string, modules map[string]map[string]string) {
	t.Helper()
	// Create go.work
	goWork := "go 1.25\n"
	for name := range modules {
		goWork += "use ./" + name + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.work"), []byte(strings.TrimSpace(goWork)), 0644); err != nil {
		t.Fatalf("failed to create go.work: %v", err)
	}
	// Create each module
	for name, files := range modules {
		moduleDir := filepath.Join(dir, name)
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("failed to create module dir: %v", err)
		}
		CreateFixtureModule(t, moduleDir, files)
	}
}

// TraverseWithOp runs the named operator over dir and returns (op, sites).
func TraverseWithOp(t *testing.T, dir, opName string) (mutator.Operator, []engine.Site) {
	t.Helper()
	op, ok := mutator.Get(opName)
	if !ok {
		t.Fatalf("operator %q not registered", opName)
	}
	eng := engine.NewEngine(false)
	eng.SetOperators([]mutator.Operator{op})
	if err := eng.Traverse(dir, nil); err != nil {
		t.Fatalf("engine.Traverse(%s): %v", dir, err)
	}
	sites := eng.Sites()
	if len(sites) == 0 {
		t.Fatalf("no mutation sites found in %s", dir)
	}
	return op, sites
}

// StatusSummary returns "killed:3 survived:2 compile_error:1" for failure messages.
func StatusSummary(mutants []gcore.Mutant) string {
	counts := make(map[string]int)
	for _, m := range mutants {
		s := m.Status
		if s == "" {
			s = "(empty)"
		}
		counts[s]++
	}
	parts := make([]string, 0, len(counts))
	for s, n := range counts {
		parts = append(parts, fmt.Sprintf("%s:%d", s, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// RunPipeline is a thin wrapper so individual tests don't repeat the long call.
func RunPipeline(t *testing.T, sites []engine.Site, op mutator.Operator, baseDir, projectRoot string, concurrent int) ([]gcore.Mutant, error) {
	t.Helper()
	return gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, []mutator.Operator{op}, []mutator.Operator{op},
		baseDir, projectRoot,
		nil, nil, concurrent, gcache.New(), nil, nil,
		logger.New(true), false, true,
		config.ExternalSuitesConfig{}, nil,
	)
}

// CorruptActiveMutantInFile finds the FIRST "activeMutantID == N" guard line
// in content and inserts a type-error statement on the immediately following
// line.
func CorruptActiveMutantInFile(content string) (string, int, bool) {
	const pfx = "activeMutantID == "
	lines := strings.Split(content, "\n")

	for lineIdx, line := range lines {
		idx := strings.Index(line, pfx)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(pfx):]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end == 0 {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(rest[:end], "%d", &id); err != nil {
			continue
		}

		// Inject on the line immediately after the guard.
		injected := fmt.Sprintf("\tvar _schemataErr_%d string = 999 // injected type error", id)
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:lineIdx+1]...)
		newLines = append(newLines, injected)
		newLines = append(newLines, lines[lineIdx+1:]...)
		return strings.Join(newLines, "\n"), id, true
	}
	return "", 0, false
}

// Truncate truncates a string to n characters.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// MutantIDs returns a slice of mutant IDs.
func MutantIDs(mutants []gcore.Mutant) []int {
	ids := make([]int, len(mutants))
	for i, m := range mutants {
		ids[i] = m.ID
	}
	return ids
}

// NewLogger creates a new logger with the specified debug flag.
func NewLogger(debug bool) *logger.Logger {
	return logger.New(debug)
}

// RunPipelineCtx is like RunPipeline but accepts a caller-supplied context,
// allowing the caller to set deadlines (e.g. for timeout tests).
func RunPipelineCtx(t *testing.T, ctx context.Context, sites []engine.Site, op mutator.Operator, baseDir, projectRoot string, concurrent int) ([]gcore.Mutant, error) {
	t.Helper()
	return gcore.TestGenerateAndRunSchemata(
		ctx,
		sites, []mutator.Operator{op}, []mutator.Operator{op},
		baseDir, projectRoot,
		nil, nil, concurrent, gcache.New(), nil, nil,
		logger.New(true), false, true,
		config.ExternalSuitesConfig{}, nil,
	)
}

// RunPipelineWithOps is like RunPipeline but accepts a full operator slice
// instead of a single operator, for multi-operator tests.
func RunPipelineWithOps(t *testing.T, sites []engine.Site, ops []mutator.Operator, baseDir, projectRoot string, concurrent int) ([]gcore.Mutant, error) {
	t.Helper()
	return gcore.TestGenerateAndRunSchemata(
		context.Background(),
		sites, ops, ops,
		baseDir, projectRoot,
		nil, nil, concurrent, gcache.New(), nil, nil,
		logger.New(true), false, true,
		config.ExternalSuitesConfig{}, nil,
	)
}

// NewEngine returns a bare engine instance for tests that need to traverse
// with multiple operators before calling GenerateMutants directly.
func NewEngine(debug bool) *engine.Engine {
	return engine.NewEngine(debug)
}
