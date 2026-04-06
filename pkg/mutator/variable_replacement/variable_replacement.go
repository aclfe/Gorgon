package variable_replacement

import (
	"go/ast"
	"go/token"
	"sync"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type VariableReplacement struct{}

var (
	funcVarsCache = make(map[*ast.FuncDecl][]string)
	cacheMu       sync.RWMutex
)

func (VariableReplacement) Name() string {
	return "variable_replacement"
}

func (VariableReplacement) CanApply(n ast.Node) bool {
	return false
}

func (VariableReplacement) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name == "_" {
		return false
	}
	if ctx.EnclosingFunc == nil {
		return false
	}
	switch p := ctx.Parent.(type) {
	case *ast.FuncDecl:
		return false
	case *ast.File:
		return false
	case *ast.Field:
		return false
	case *ast.SelectorExpr:
		if p.Sel == ident {
			return false
		}
	case *ast.KeyValueExpr:
		if p.Key == ident {
			return false
		}
	case *ast.AssignStmt:
		for _, lhs := range p.Lhs {
			if lhs == ident {
				return false
			}
		}
	case *ast.IncDecStmt:
		return false
	case *ast.RangeStmt:
		if p.Key == ident || p.Value == ident {
			return false
		}
	case *ast.TypeAssertExpr:
		if p.Type == ident {
			return false
		}
	case *ast.StarExpr:
		return false
	case *ast.ArrayType:
		return false
	case *ast.MapType:
		return false
	case *ast.ChanType:
		return false
	case *ast.InterfaceType:
		return false
	case *ast.StructType:
		return false
	case *ast.FuncType:
		return false
	case *ast.CompositeLit:
		if p.Type == ident {
			return false
		}
	}
	return true
}

func (VariableReplacement) Mutate(n ast.Node) ast.Node {
	return nil
}

func (VariableReplacement) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ident, ok := n.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return nil
	}
	if ctx.EnclosingFunc == nil {
		return nil
	}
	replacement := findReplacementVar(ctx.EnclosingFunc, ident.Name)
	if replacement == "" || replacement == ident.Name {
		return nil
	}
	return &ast.Ident{NamePos: ident.NamePos, Name: replacement}
}

func findReplacementVar(fn *ast.FuncDecl, exclude string) string {
	cacheMu.RLock()
	cached, ok := funcVarsCache[fn]
	cacheMu.RUnlock()

	if ok {
		for _, v := range cached {
			if v != exclude {
				return v
			}
		}
		return ""
	}

	// Estimate capacity from function signature (params + results)
	cap := 0
	if fn.Type.Params != nil {
		for _, f := range fn.Type.Params.List {
			cap += len(f.Names)
		}
	}
	if fn.Type.Results != nil {
		for _, f := range fn.Type.Results.List {
			cap += len(f.Names)
		}
	}
	// Add generous estimate for body declarations (typical function has 2-10)
	cap += 8
	candidates := make([]string, 0, cap)

	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				if name.Name != "_" {
					candidates = append(candidates, name.Name)
				}
			}
		}
	}

	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			for _, name := range field.Names {
				if name.Name != "_" {
					candidates = append(candidates, name.Name)
				}
			}
		}
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name != "_" {
						candidates = append(candidates, ident.Name)
					}
				}
			}
		}
		if decl, ok := n.(*ast.DeclStmt); ok {
			if gen, ok := decl.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if name.Name != "_" {
								candidates = append(candidates, name.Name)
							}
						}
					}
				}
			}
		}
		return true
	})

	cacheMu.Lock()
	funcVarsCache[fn] = candidates
	cacheMu.Unlock()

	for _, v := range candidates {
		if v != exclude {
			return v
		}
	}
	return ""
}

func init() {
	mutator.Register(VariableReplacement{})
}

var _ mutator.Operator = VariableReplacement{}
var _ mutator.ContextualOperator = VariableReplacement{}
