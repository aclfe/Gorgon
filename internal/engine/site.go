package engine

import (
	"go/ast"
	"go/token"
)

type Site struct {
	File *token.File
	Pos  token.Pos
	End  token.Pos
	Node ast.Node
}
