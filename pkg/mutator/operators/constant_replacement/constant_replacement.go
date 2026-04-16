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
	bl, ok := n.(*ast.BasicLit)
	if !ok {
		return false
	}

	p := ctx.Parent

	switch parent := p.(type) {

	case *ast.AssignStmt:
		for _, rhs := range parent.Rhs {
			if rhs == bl {

				if parent.Tok == token.DEFINE || parent.Tok == token.ASSIGN {
					return isSafeLiteral(bl)
				}
			}
		}
		return false

	case *ast.ReturnStmt:
		for _, r := range parent.Results {
			if r == bl {
				return isSafeLiteral(bl)
			}
		}
		return false

	case *ast.BinaryExpr:
		if isTimeDuration(parent.X) || isTimeDuration(parent.Y) {
			return false
		}

		switch parent.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ,
			token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
			return isSafeLiteral(bl)
		}
		return false

	case *ast.ValueSpec:
		for _, v := range parent.Values {
			if v == bl {

				if parent.Type == nil {
					return isSafeLiteral(bl)
				}

				return false
			}
		}
		return false

	case *ast.CompositeLit:
		for _, elt := range parent.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {

				if kv.Value == bl {
					return isSafeLiteral(bl)
				}
			} else if elt == bl {
				return isSafeLiteral(bl)
			}
		}
		return false

	case *ast.ExprStmt:
		return false

	default:
		return false
	}
}

func isSafeLiteral(bl *ast.BasicLit) bool {
	switch bl.Kind {
	case token.INT:

		return true
	case token.FLOAT:
		return true
	case token.STRING:
		return true
	case token.CHAR:
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

func (ConstantReplacement) RequiresTypeCheck() bool {
	return true
}
