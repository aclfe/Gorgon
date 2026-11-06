package engine

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/internal/core/schemata_nodes"
)

type Site struct {
	File      *token.File
	FileAST   *ast.File
	Fset      *token.FileSet
	Line      int
	Column    int
	Offset    int
	EndOffset int
	Node      ast.Node
	Parent    ast.Node

	ReturnType    string
	FunctionName  string
	EnclosingFunc *ast.FuncDecl

	ExprType string

	Ops []string

	EvaluatedOps map[string]bool

	Directives map[int]map[string]map[int]bool

	Hints map[string]string
}

type SiteKey struct {
	File      string
	Offset    int
	EndOffset int
	NodeType  uint8
}

func (s Site) Key() SiteKey {
	name := ""
	if s.File != nil {
		name = s.File.Name()
	}
	return SiteKey{
		File:      name,
		Offset:    s.Offset,
		EndOffset: s.EndOffset,
		NodeType:  schemata_nodes.NodeTypeToUint8(s.Node),
	}
}
