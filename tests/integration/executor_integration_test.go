//go:build integration
// +build integration

package integration

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	coretesting "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
)

// TestResolveSuitePaths_ResolvesRealPackage verifies that resolveSuitePaths
// runs go list correctly and returns the relative path for a real package.
func TestResolveSuitePaths_ResolvesRealPackage(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")

	suite := config.ExternalSuite{
		Name:  "test-suite",
		Paths: []string{"./internal/logger/..."},
	}

	log := logger.New(false)
	ctx := context.Background()

	paths, err := coretesting.TestResolveSuitePaths(ctx, repoRoot, suite, log)
	if err != nil {
		t.Fatalf("TestResolveSuitePaths: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one resolved path, got none")
	}
	for _, p := range paths {
		if !strings.HasPrefix(p, "./") {
			t.Errorf("resolved path %q should start with ./", p)
		}
	}
}
