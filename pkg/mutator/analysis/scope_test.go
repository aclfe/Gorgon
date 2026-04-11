package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseFunc(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatal("no function found in source")
	return nil
}

func parseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", "package p; var _ = "+src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0]
}

func parseTypeExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", "package p; var _ "+src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Type
}

func TestCollectVars_ParamsAndReturns(t *testing.T) {
	fn := parseFunc(t, `package p
func f(a int, b string) (result bool) {}`)
	vars := CollectVars(fn)
	if len(vars) != 3 {
		t.Fatalf("expected 3 vars, got %d", len(vars))
	}
	expect := []VarInfo{
		{Name: "a", Type: "int"},
		{Name: "b", Type: "string"},
		{Name: "result", Type: "bool"},
	}
	for i, e := range expect {
		if vars[i] != e {
			t.Errorf("var[%d]: got %+v, want %+v", i, vars[i], e)
		}
	}
}

func TestCollectVars_BodyDecls(t *testing.T) {
	fn := parseFunc(t, `package p
func f(x int) int {
	y := x + 1
	var z string
	return y
}`)
	vars := CollectVars(fn)
	if len(vars) != 3 {
		t.Fatalf("expected 3 vars, got %d", len(vars))
	}
	found := make(map[string]string)
	for _, v := range vars {
		found[v.Name] = v.Type
	}
	if found["y"] != "int" {
		t.Errorf("y type: got %q, want %q", found["y"], "int")
	}
	if found["z"] != "string" {
		t.Errorf("z type: got %q, want %q", found["z"], "string")
	}
}

func TestCollectVars_IgnoresNestedBlocks(t *testing.T) {
	fn := parseFunc(t, `package p
func f() {
	if true {
		nested := 1
		_ = nested
	}
}`)
	vars := CollectVars(fn)
	if len(vars) != 0 {
		t.Errorf("expected 0 vars, got %d", len(vars))
	}
}

func TestBuildTypeMap(t *testing.T) {
	fn := parseFunc(t, `package p
func f(a int, b string) bool {
	c := a
	_ = c
	return true
}`)
	m := BuildTypeMap(fn)
	if m["a"] != "int" {
		t.Errorf("a: got %q, want %q", m["a"], "int")
	}
	if m["b"] != "string" {
		t.Errorf("b: got %q, want %q", m["b"], "string")
	}
	if m["c"] != "int" {
		t.Errorf("c: got %q, want %q", m["c"], "int")
	}
}

func TestTypesCompatible(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"int", "int", true},
		{"string", "string", true},
		{"int", "string", false},
		{"int", "int64", false},
		{"", "int", false},
		{"int", "", false},
		{"func:opaque", "int", false},
		{"int", "interface{}", false},
		{"interface{}", "interface{}", false},
		{"*int", "*int", true},
		{"[]byte", "[]byte", true},
	}
	for _, tt := range tests {
		if got := TypesCompatible(tt.a, tt.b); got != tt.want {
			t.Errorf("TypesCompatible(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFindCompatibleVar(t *testing.T) {
	vars := []VarInfo{
		{Name: "a", Type: "int"},
		{Name: "b", Type: "string"},
		{Name: "c", Type: "int"},
	}
	if v := FindCompatibleVar(vars, "a", "int"); v.Name != "c" {
		t.Errorf("got %q, want %q", v.Name, "c")
	}
	if v := FindCompatibleVar(vars, "b", "string"); v.Name != "" {
		t.Errorf("got %q, want empty", v.Name)
	}
	if v := FindCompatibleVar(vars, "a", ""); v.Name != "" {
		t.Errorf("got %q, want empty", v.Name)
	}
}

func TestResolveType_Literals(t *testing.T) {
	tests := []struct{ src, want string }{
		{`42`, "int"}, {`3.14`, "float64"}, {`"hello"`, "string"},
		{`'x'`, "rune"}, {`true`, "bool"}, {`!x`, "bool"},
	}
	typeMap := map[string]string{"x": "bool"}
	for _, tt := range tests {
		got := ResolveType(parseExpr(t, tt.src), typeMap)
		if got != tt.want {
			t.Errorf("ResolveType(%s) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestTypeExprToString(t *testing.T) {
	tests := []struct{ src, want string }{
		{`int`, "int"}, {`*int`, "*int"}, {`[]string`, "[]string"},
		{`map[string]int`, "map[string]int"}, {`chan bool`, "chan bool"},
		{`interface{}`, "interface{}"}, {`struct{}`, "struct{}"},
		{`pkg.Type`, "pkg.Type"},
	}
	for _, tt := range tests {
		got := TypeExprToString(parseTypeExpr(t, tt.src))
		if got != tt.want {
			t.Errorf("TypeExprToString(%s) = %q, want %q", tt.src, got, tt.want)
		}
	}
}
