package testing

import (
	"context"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)

func RunMutants(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int, cache *cache.Cache, tests []string) ([]Mutant, error) {
	return GenerateAndRunSchemata(ctx, sites, operators, baseDir, concurrent, cache, tests)
}
