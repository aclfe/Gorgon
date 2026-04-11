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

func (ConstantReplacement) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	if !(ConstantReplacement{}).CanApply(n) {
		return false
	}

	bl := n.(*ast.BasicLit)
	p := ctx.Parent

	switch p := p.(type) {
	case *ast.ArrayType:
		if p.Len == bl {
			return false
		}
	case *ast.CallExpr:
		if len(p.Args) > 0 && p.Args[0] == bl {
			if sel, ok := p.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "fmt" && sel.Sel.Name == "Errorf" {
					return false
				}
			}
		}
	}

	if bin, ok := p.(*ast.BinaryExpr); ok && bin.Op == token.MUL {
		if isTimeDuration(bin.X) || isTimeDuration(bin.Y) {
			return false
		}
	}

	return true
}

func (ConstantReplacement) Mutate(n ast.Node) ast.Node {
    bl, ok := n.(*ast.BasicLit)
    if !ok {
        return nil
    }
    return mutateBasicLit(bl)
}

func (ConstantReplacement) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
    cr := ConstantReplacement{}
    if !cr.CanApplyWithContext(n, ctx) {
        return nil
    }
    return cr.Mutate(n)
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
		if isEmptyStringLiteral(bl.Value) {
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

func isEmptyStringLiteral(value string) bool {
	if len(value) != 2 {
		return false
	}
	first := value[0]
	return (first == '"' || first == '`') && value[1] == first
}

func isTimeDuration(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "time" {
		return false
	}
	switch sel.Sel.Name {
	case "Nanosecond", "Microsecond", "Millisecond", "Second", "Minute", "Hour":
		return true
	}
	return false
}

func init() {
	mutator.Register(ConstantReplacement{})
}

var _ mutator.Operator = ConstantReplacement{}
var _ mutator.ContextualOperator = ConstantReplacement{}