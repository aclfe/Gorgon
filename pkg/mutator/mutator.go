// Package mutator provides mutation operators for the gorgon project
package mutator

import (
	"go/ast"
)

type Operator interface {
	Name() string
	CanApply(node ast.Node) bool
	Mutate(node ast.Node) string
}
