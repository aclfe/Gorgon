package constant_replacement

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ConstantReplacement struct{}

func (ConstantReplacement) Name() string {
	return "constant_replacement"
}

func (ConstantReplacement) CanApply(n ast.Node) bool {
	bl, ok := n.(*ast.BasicLit)
	if !ok {
		return false
	}
	switch bl.Kind {
	case token.INT, token.FLOAT, token.STRING, token.CHAR:
		return true
	}
	return false
}

func (ConstantReplacement) Mutate(n ast.Node) ast.Node {
	bl, ok := n.(*ast.BasicLit)
	if !ok {
		return nil
	}
	return mutateBasicLit(bl)
}

func mutateBasicLit(bl *ast.BasicLit) ast.Node {
	switch bl.Kind {
	case token.INT:
		if bl.Value == "0" {
			return &ast.BasicLit{Kind: token.INT, Value: "1"}
		}
		return &ast.BasicLit{Kind: token.INT, Value: "0"}
	case token.FLOAT:
		if bl.Value == "0.0" {
			return &ast.BasicLit{Kind: token.FLOAT, Value: "1.0"}
		}
		return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}
	case token.STRING:
		if bl.Value == `""` || bl.Value == "\"\"" {
			return &ast.BasicLit{Kind: token.STRING, Value: `" "`}
		}
		return &ast.BasicLit{Kind: token.STRING, Value: `""`}
	case token.CHAR:
		if bl.Value == "'a'" {
			return &ast.BasicLit{Kind: token.CHAR, Value: "'b'"}
		}
		return &ast.BasicLit{Kind: token.CHAR, Value: "'a'"}
	}
	return nil
}

func init() {
	mutator.Register(ConstantReplacement{})
}

var _ mutator.Operator = ConstantReplacement{}
