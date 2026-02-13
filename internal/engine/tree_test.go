package engine_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
)

//nolint:varnamelen,paralleltest // short vars idiomatic in tests; cannot run in parallel - modifies global PrintEnabled
func TestPrintTree(t *testing.T) {
	path := "../../test/testdata/astprint/consolidated.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	var buf bytes.Buffer
	engine.PrintEnabled = true
	if err := engine.PrintTree(&buf, fset, f); err != nil {
		t.Fatalf("PrintTree failed: %v", err)
	}

	output := buf.String()

	expectedNodes := []string{
		"File",
		"FuncDecl",
		"functionWithParams",
		"GenDecl",
		"StructType",
		"InterfaceType",
		"GoStmt",
		"DeferStmt",
		"SelectStmt",
		"RangeStmt",
	}

	for _, node := range expectedNodes {
		if !strings.Contains(output, node) {
			t.Errorf("Output missing expected node type/name: %s", node)
		}
	}
}

//nolint:paralleltest // cannot run in parallel - modifies global PrintEnabled state
func TestPrintTreeDisabled(t *testing.T) {
	engine.PrintEnabled = false
	var buf bytes.Buffer
	if err := engine.PrintTree(&buf, nil, nil); err != nil {
		t.Fatalf("PrintTree failed: %v", err)
	}
	if buf.Len() > 0 {
		t.Error("PrintTree output something when PrintEnabled is false")
	}
}
