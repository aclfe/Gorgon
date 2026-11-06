package schemata_nodes

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
)

var posType = reflect.TypeOf(token.NoPos)

func parseTypeExpr(typ string) (ast.Expr, error) {
	if typ == "" {
		return nil, fmt.Errorf("empty type")
	}
	expr, err := parser.ParseExpr(typ)
	if err != nil {
		return nil, err
	}
	StripPositions(expr)
	return expr, nil
}

func StripPositions(n ast.Node) {
	ast.Inspect(n, func(x ast.Node) bool {
		if x == nil {
			return false
		}
		v := reflect.ValueOf(x)
		if v.Kind() != reflect.Pointer || v.IsNil() {
			return true
		}
		v = v.Elem()
		if v.Kind() != reflect.Struct {
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Type() == posType && f.CanSet() {
				f.SetInt(0)
			}
		}
		return true
	})
}

func FillPositions(n ast.Node, pos token.Pos) {
	if n == nil || !pos.IsValid() {
		return
	}
	ast.Inspect(n, func(x ast.Node) bool {
		if x == nil {
			return false
		}
		v := reflect.ValueOf(x)
		if v.Kind() != reflect.Pointer || v.IsNil() {
			return true
		}
		v = v.Elem()
		if v.Kind() != reflect.Struct {
			return true
		}
		hasPos := false
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Type() == posType && f.Int() != 0 {
				hasPos = true
				break
			}
		}
		if hasPos {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Type() != posType || !f.CanSet() {
				continue
			}
			switch t.Field(i).Name {
			case "Ellipsis", "Arrow":
				continue
			}
			f.SetInt(int64(pos))
		}
		return true
	})
}
