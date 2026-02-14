package engine_test

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
)

func TestTraverse(t *testing.T) {
	t.Parallel()
	// Test traversing the astprint directory which contains consolidated.go
	path := "../../test/testdata/astprint"
	count := 0
	err := engine.Traverse(path, func(_ ast.Node) bool {
		count++
		return true
	})
	if err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}

	if count == 0 {
		t.Fatal("Traverse visited 0 nodes, expected > 0")
	}
}

func TestTraverseSingleFile(t *testing.T) {
	t.Parallel()
	path := "../../test/testdata/astprint/consolidated.go"
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	visited := false
	err = engine.Traverse(path, func(_ ast.Node) bool {
		visited = true
		return true
	})
	if err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}

	if !visited {
		t.Fatal("Traverse did not visit any nodes in consolidated.go")
	}
}

func TestTraverseError(t *testing.T) {
	t.Parallel()
	err := engine.Traverse("non_existent_file.go", func(_ ast.Node) bool { return true })
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestTraverse_NotGoFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "readme.txt")
	const fileMode = 0o600
	if err := os.WriteFile(path, []byte("hello"), fileMode); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	err := engine.Traverse(path, func(_ ast.Node) bool {
		t.Fatal("Visitor should not be called for non-go file")
		return true
	})
	if err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}
}

func TestTraverse_DirError(t *testing.T) {
	t.Parallel()
	err := engine.Traverse("non_existent_dir", func(_ ast.Node) bool { return true })
	if err == nil {
		t.Fatal("Expected error for non-existent dir")
	}
}

//nolint:paralleltest // cannot run in parallel - modifies global PrintEnabled state
func TestTraverseSingleFileWithPrint(t *testing.T) {
	// Cannot run in parallel - modifies global PrintEnabled state
	originalPrintEnabled := engine.PrintEnabled
	defer func() { engine.PrintEnabled = originalPrintEnabled }()
	engine.PrintEnabled = true

	path := "../../test/testdata/astprint/print.go"
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	visited := false
	err = engine.Traverse(path, func(_ ast.Node) bool {
		visited = true
		return true
	})
	if err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}

	if !visited {
		t.Fatal("Traverse did not visit any nodes")
	}
}
