package engine

import (
	"go/ast"
	"go/token"
)

type Site struct {
	File          *token.File
	FileAST       *ast.File
	Fset          *token.FileSet
	Line          int
	Column        int
	Node          ast.Node
	ReturnType    string
	FunctionName  string
	EnclosingFunc *ast.FuncDecl
}
