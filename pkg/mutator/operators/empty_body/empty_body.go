package empty_body

import (
	"go/ast"
	"unicode"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type EmptyBody struct{}

func (EmptyBody) Name() string {
	return "empty_body"
}

func (EmptyBody) CanApply(n ast.Node) bool {
	return false 
}

func (EmptyBody) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	fn, ok := n.(*ast.FuncDecl)
	if !ok {
		return false
	}

	
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}

	
	
	if fn.Name != nil {
		switch fn.Name.Name {
		case "init", "main":
			return false
		}
	}

	
	
	if fn.Name != nil && isExported(fn.Name.Name) {
		return false
	}

	
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		return false
	}

	
	
	
	if len(fn.Body.List) < 2 {
		return false
	}

	
	
	
	
	
	if fn.Recv != nil && fn.Name != nil && !isExported(fn.Name.Name) {
		
		
		if !hasConcretReceiver(fn) {
			return false
		}
	}

	return true
}

func (EmptyBody) Mutate(n ast.Node) ast.Node {
	return nil 
}

func (EmptyBody) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	fn, ok := n.(*ast.FuncDecl)
	if !ok {
		return nil
	}
	if !(EmptyBody{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	return &ast.FuncDecl{
		Doc:  fn.Doc,
		Recv: fn.Recv,
		Name: fn.Name,
		Type: fn.Type,
		Body: &ast.BlockStmt{
			Lbrace: fn.Body.Lbrace,
			List:   []ast.Stmt{},
			Rbrace: fn.Body.Rbrace,
		},
	}
}


func isExported(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}



func hasConcretReceiver(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return false
	}
	field := fn.Recv.List[0]
	switch t := field.Type.(type) {
	case *ast.Ident:
		return t.Name != "_" && t.Name != ""
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name != "_" && id.Name != ""
		}
	}
	return false
}

func init() {
	mutator.Register(EmptyBody{})
}

var _ mutator.Operator = EmptyBody{}
var _ mutator.ContextualOperator = EmptyBody{}