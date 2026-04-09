package testing_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	gtest "github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/logical_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_body_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_break_first"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/variable_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/zero_value_return"
)

const benchDir = "../../examples/mutations/arithmetic_flip"

// =============================================================================
// GenerateMutants: NEW (single-pass) vs OLD (sort + dedup + generate)
// =============================================================================

func BenchmarkGenerateMutants_New(b *testing.B) {
	sites := loadSites(b, benchDir)
	ops := []mutator.Operator{mutator.MustGet("arithmetic_flip")}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mutants := gtest.GenerateMutants(sites, ops)
		if len(mutants) == 0 {
			b.Fatal("no mutants")
		}
	}
}

func BenchmarkGenerateMutants_Old(b *testing.B) {
	sites := loadSites(b, benchDir)
	ops := []mutator.Operator{mutator.MustGet("arithmetic_flip")}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// OLD way: sort first, then dedup, then generate
		sorted := make([]engine.Site, len(sites))
		copy(sorted, sites)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].File.Name() < sorted[j].File.Name()
		})

		type siteKey struct {
			file string
			line int
			col  int
			ntyp uint8
		}
		seen := make(map[siteKey]bool)
		var unique []engine.Site
		for _, site := range sorted {
			key := siteKey{
				file: site.File.Name(),
				line: site.Line,
				col:  site.Column,
				ntyp: schemata_nodes.NodeTypeToUint8(site.Node),
			}
			if !seen[key] {
				seen[key] = true
				unique = append(unique, site)
			}
		}

		var mutants []gtest.Mutant
		mutantID := 1
		for _, site := range unique {
			for _, op := range ops {
				apply := false
				if cop, ok := op.(mutator.ContextualOperator); ok {
					ctx := mutator.Context{ReturnType: site.ReturnType}
					apply = cop.CanApplyWithContext(site.Node, ctx)
				} else {
					apply = op.CanApply(site.Node)
				}
				if apply {
					mutants = append(mutants, gtest.Mutant{
						ID:       mutantID,
						Site:     site,
						Operator: op,
					})
					mutantID++
				}
			}
		}
		if len(mutants) == 0 {
			b.Fatal("no mutants")
		}
	}
}

// =============================================================================
// ResolveCache: batched hashing vs per-mutant hashing
// =============================================================================

func BenchmarkResolveCache_AllUncached(b *testing.B) {
	mutants := buildMutantsForCacheBench(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset statuses
		for j := range mutants {
			mutants[j].Status = ""
		}

		toRun, _, err := gtest.ResolveCache(mutants, "", nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(toRun) != len(mutants) {
			b.Fatalf("expected %d uncached, got %d", len(mutants), len(toRun))
		}
	}
}

func BenchmarkResolveCache_WithCache(b *testing.B) {
	mutants := buildMutantsForCacheBench(b)
	c := cache.New()
	_ = c.Save(b.TempDir()) // Create empty cache file

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range mutants {
			mutants[j].Status = ""
		}

		// Pre-populate cache for some mutants
		c2 := cache.New()
		half := len(mutants) / 2
		for j := 0; j < half; j++ {
			mutants[j].Status = "killed"
		}

		toRun, _, err := gtest.ResolveCache(mutants, "", c2)
		if err != nil {
			b.Fatal(err)
		}
		// Should skip the pre-populated ones
		_ = toRun
	}
}

// =============================================================================
// SaveCache: batched vs per-mutant file hashing
// =============================================================================

func BenchmarkSaveCache_Batched(b *testing.B) {
	mutants := buildMutantsForCacheBench(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gtest.SaveCache(mutants, b.TempDir(), cache.New(), nil)
	}
}

// =============================================================================
// Full pipeline benchmark (replaces commented-out old benchmark)
// =============================================================================

func BenchmarkFullPipeline(b *testing.B) {
	sites := loadSites(b, benchDir)
	ops := []mutator.Operator{mutator.MustGet("arithmetic_flip")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, benchDir, 1, nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Schemata failed: %v", err)
		}
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func loadSites(t testing.TB, basePath string) []engine.Site {
	t.Helper()

	absPath, err := filepath.Abs(basePath)
	if err != nil {
		t.Fatal(err)
	}

	var sites []engine.Site
	fset := token.NewFileSet()

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				return true
			}
			pos := fset.Position(n.Pos())
			sites = append(sites, engine.Site{
				File:   fset.File(n.Pos()),
				Line:   pos.Line,
				Column: pos.Column,
				Node:   n,
			})
			return true
		})
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
	return sites
}

func buildMutantsForCacheBench(t testing.TB) []gtest.Mutant {
	t.Helper()
	sites := loadSites(t, benchDir)
	ops := []mutator.Operator{mutator.MustGet("arithmetic_flip")}
	return gtest.GenerateMutants(sites, ops)
}
