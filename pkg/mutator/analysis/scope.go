package analysis

import (
	"go/ast"
	"go/token"
)

type VarInfo struct {
	Name string
	Type string
}

func CollectVars(fn *ast.FuncDecl) []VarInfo {
	if fn == nil {
		return nil
	}
	var vars []VarInfo

	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			typ := TypeExprToString(field.Type)
			for _, name := range field.Names {
				if name.Name != "_" {
					vars = append(vars, VarInfo{Name: name.Name, Type: typ})
				}
			}
		}
	}

	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			typ := TypeExprToString(field.Type)
			for _, name := range field.Names {
				if name.Name != "_" {
					vars = append(vars, VarInfo{Name: name.Name, Type: typ})
				}
			}
		}
	}

	typeMap := make(map[string]string, len(vars))
	for _, v := range vars {
		if v.Type != "" {
			typeMap[v.Name] = v.Type
		}
	}

	if fn.Body != nil {
		for _, stmt := range fn.Body.List {
			vars = collectTopLevelVars(stmt, typeMap, vars)
		}
	}

	return vars
}

func BuildTypeMap(fn *ast.FuncDecl) map[string]string {
	vars := CollectVars(fn)
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		if v.Type != "" {
			m[v.Name] = v.Type
		}
	}
	return m
}

func collectTopLevelVars(stmt ast.Stmt, typeMap map[string]string, vars []VarInfo) []VarInfo {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok != token.DEFINE {
			break
		}
		for i, lhs := range s.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			var rhs ast.Expr
			if i < len(s.Rhs) {
				rhs = s.Rhs[i]
			}
			typ := ResolveType(rhs, typeMap)
			if typ != "" {
				typeMap[id.Name] = typ
			}
			vars = append(vars, VarInfo{Name: id.Name, Type: typ})
		}
	case *ast.DeclStmt:
		gen, ok := s.Decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			break
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			declaredType := ""
			if vs.Type != nil {
				declaredType = TypeExprToString(vs.Type)
			}
			for i, name := range vs.Names {
				if name.Name == "_" {
					continue
				}
				typ := declaredType
				if typ == "" && i < len(vs.Values) {
					typ = ResolveType(vs.Values[i], typeMap)
				}
				if typ != "" {
					typeMap[name.Name] = typ
				}
				vars = append(vars, VarInfo{Name: name.Name, Type: typ})
			}
		}
	}
	return vars
}

func ResolveType(expr ast.Expr, typeMap map[string]string) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if typ, ok := typeMap[e.Name]; ok {
			return typ
		}
		switch e.Name {
		case "true", "false":
			return "bool"
		}
		return ""
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "rune"
		}
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return "bool"
		}
		return ResolveType(e.X, typeMap)
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ,
			token.LAND, token.LOR:
			return "bool"
		}
		if t := ResolveType(e.X, typeMap); t != "" {
			return t
		}
		return ResolveType(e.Y, typeMap)
	case *ast.CallExpr:
		return ""
	case *ast.ParenExpr:
		return ResolveType(e.X, typeMap)
	case *ast.StarExpr:
		inner := ResolveType(e.X, typeMap)
		if inner == "" {
			return ""
		}
		return "*" + inner
	default:
		return TypeExprToString(expr)
	}
	return ""
}

func TypeExprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + TypeExprToString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + TypeExprToString(e.Elt)
		}
		return "[" + constExprToString(e.Len) + "]" + TypeExprToString(e.Elt)
	case *ast.MapType:
		return "map[" + TypeExprToString(e.Key) + "]" + TypeExprToString(e.Value)
	case *ast.ChanType:
		return "chan " + TypeExprToString(e.Value)
	case *ast.FuncType:
		return "func:opaque"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.ParenExpr:
		return TypeExprToString(e.X)
	}
	return ""
}

func constExprToString(expr ast.Expr) string {
	if bl, ok := expr.(*ast.BasicLit); ok {
		return bl.Value
	}
	return ""
}

func TypesCompatible(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == "func:opaque" || b == "func:opaque" {
		return false
	}
	if a == "interface{}" || b == "interface{}" {
		return false
	}
	return a == b
}

func FindCompatibleVar(vars []VarInfo, exclude, targetType string) VarInfo {
	if targetType == "" {
		return VarInfo{}
	}
	for _, v := range vars {
		if v.Name == exclude {
			continue
		}
		if v.Type != "" && TypesCompatible(v.Type, targetType) {
			return v
		}
	}
	return VarInfo{}
}

func HasCompatibleVar(vars []VarInfo, exclude, targetType string) bool {
	if targetType == "" {
		return false
	}
	for _, v := range vars {
		if v.Name == exclude {
			continue
		}
		if v.Type != "" && TypesCompatible(v.Type, targetType) {
			return true
		}
	}
	return false
}
