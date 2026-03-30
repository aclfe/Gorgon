package engine

import (
	"go/ast"
	"go/token"
)

type Site struct {
	File       *token.File
	Line       int
	Column     int
	Node       ast.Node
	ReturnType string
}
