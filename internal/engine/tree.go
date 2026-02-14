package engine

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"reflect"
	"strings"
)

const astPrefix = "*ast."

// PrintEnabled controls whether AST trees are printed during traversal.
// Linter forced again.
var PrintEnabled bool

var (
	nodeKindMap  = make(map[reflect.Type]string)
	descFuncMap  = make(map[reflect.Type]func(ast.Node) string)
	childFuncMap = make(map[reflect.Type]func(ast.Node) []ast.Node)
)

func init() {
	registerFileAndFuncNodes()
	registerDeclAndSpecNodes()
	registerStmtNodes()
	registerExprNodes()
	registerTypeNodes()
	registerMiscNodes()
}

//nolint:gocognit
func registerFileAndFuncNodes() {
	registerNode(&ast.File{}, "File", func(_ *ast.File) string { return "" }, func(fileNode *ast.File) []ast.Node {
		children := []ast.Node{}
		if fileNode.Comments != nil {
			for _, cg := range fileNode.Comments {
				children = append(children, cg)
			}
		}
		if fileNode.Name != nil {
			children = append(children, fileNode.Name)
		}
		return append(children, anySliceToNodes(fileNode.Decls)...)
	})
	registerNode(&ast.FuncDecl{}, "FuncDecl", func(funcDecl *ast.FuncDecl) string {
		return "Function/Method: " + funcDecl.Name.Name
	}, func(funcDecl *ast.FuncDecl) []ast.Node {
		children := []ast.Node{}
		if funcDecl.Name != nil {
			children = append(children, funcDecl.Name)
		}
		if funcDecl.Type != nil {
			children = append(children, funcDecl.Type)
		}
		if funcDecl.Body != nil {
			children = append(children, funcDecl.Body)
		}
		return children
	})
	registerNode(&ast.FuncType{}, "FuncType",
		func(_ *ast.FuncType) string { return "" },
		func(funcType *ast.FuncType) []ast.Node {
			children := []ast.Node{}
			if funcType.TypeParams != nil {
				children = append(children, funcType.TypeParams)
			}
			if funcType.Params != nil {
				children = append(children, funcType.Params)
			}
			if funcType.Results != nil {
				children = append(children, funcType.Results)
			}
			return children
		})
	registerNode(&ast.Field{}, "Field", func(_ *ast.Field) string { return "" }, func(field *ast.Field) []ast.Node {
		children := anySliceToNodes(field.Names)
		if field.Type != nil {
			children = append(children, field.Type)
		}
		if field.Tag != nil {
			children = append(children, field.Tag)
		}
		return children
	})
}

func registerDeclAndSpecNodes() {
	registerNode(&ast.GenDecl{}, "GenDecl", func(genDecl *ast.GenDecl) string {
		return fmt.Sprintf("Decl Type: %s", genDecl.Tok)
	}, func(genDecl *ast.GenDecl) []ast.Node {
		return anySliceToNodes(genDecl.Specs)
	})
	registerNode(&ast.ValueSpec{}, "ValueSpec", func(valSpec *ast.ValueSpec) string {
		strBuilder := strings.Builder{}
		for _, name := range valSpec.Names {
			strBuilder.WriteString(fmt.Sprintf("Var/Const: %s ", name.Name))
		}
		return strBuilder.String()
	}, func(valSpec *ast.ValueSpec) []ast.Node {
		children := anySliceToNodes(valSpec.Names)
		if valSpec.Type != nil {
			children = append(children, valSpec.Type)
		}
		return append(children, anySliceToNodes(valSpec.Values)...)
	})
	registerNode(&ast.TypeSpec{}, "TypeSpec", func(typeSpec *ast.TypeSpec) string {
		return "Type Name: " + typeSpec.Name.Name
	}, func(typeSpec *ast.TypeSpec) []ast.Node {
		children := []ast.Node{}
		if typeSpec.Name != nil {
			children = append(children, typeSpec.Name)
		}
		if typeSpec.Type != nil {
			children = append(children, typeSpec.Type)
		}
		return children
	})
}

//nolint:gocyclo,gocognit,cyclop,funlen
func registerStmtNodes() {
	registerNode(&ast.BlockStmt{}, "BlockStmt",
		func(_ *ast.BlockStmt) string { return "" },
		func(block *ast.BlockStmt) []ast.Node {
			return anySliceToNodes(block.List)
		})
	registerNode(
		&ast.IfStmt{},
		"IfStmt",
		func(_ *ast.IfStmt) string { return "" },
		func(ifStmt *ast.IfStmt) []ast.Node {
			children := []ast.Node{}
			if ifStmt.Init != nil {
				children = append(children, ifStmt.Init)
			}
			if ifStmt.Cond != nil {
				children = append(children, ifStmt.Cond)
			}
			if ifStmt.Body != nil {
				children = append(children, ifStmt.Body)
			}
			if ifStmt.Else != nil {
				children = append(children, ifStmt.Else)
			}
			return children
		},
	)
	registerNode(
		&ast.ForStmt{},
		"ForStmt",
		func(_ *ast.ForStmt) string { return "" },
		func(forStmt *ast.ForStmt) []ast.Node {
			children := []ast.Node{}
			if forStmt.Init != nil {
				children = append(children, forStmt.Init)
			}
			if forStmt.Cond != nil {
				children = append(children, forStmt.Cond)
			}
			if forStmt.Post != nil {
				children = append(children, forStmt.Post)
			}
			if forStmt.Body != nil {
				children = append(children, forStmt.Body)
			}
			return children
		},
	)
	registerNode(&ast.RangeStmt{}, "RangeStmt", func(rangeStmt *ast.RangeStmt) string {
		return fmt.Sprintf("Range Op: %s", rangeStmt.Tok)
	}, func(rangeStmt *ast.RangeStmt) []ast.Node {
		children := []ast.Node{}
		if rangeStmt.Key != nil {
			children = append(children, rangeStmt.Key)
		}
		if rangeStmt.Value != nil {
			children = append(children, rangeStmt.Value)
		}
		if rangeStmt.X != nil {
			children = append(children, rangeStmt.X)
		}
		if rangeStmt.Body != nil {
			children = append(children, rangeStmt.Body)
		}
		return children
	})
	registerNode(&ast.SwitchStmt{}, "SwitchStmt",
		func(_ *ast.SwitchStmt) string { return "" },
		func(switchStmt *ast.SwitchStmt) []ast.Node {
			children := []ast.Node{}
			if switchStmt.Init != nil {
				children = append(children, switchStmt.Init)
			}
			if switchStmt.Tag != nil {
				children = append(children, switchStmt.Tag)
			}
			if switchStmt.Body != nil {
				children = append(children, switchStmt.Body)
			}
			return children
		})
	registerNode(&ast.TypeSwitchStmt{}, "TypeSwitchStmt",
		func(_ *ast.TypeSwitchStmt) string { return "" },
		func(typeSwitch *ast.TypeSwitchStmt) []ast.Node {
			children := []ast.Node{}
			if typeSwitch.Init != nil {
				children = append(children, typeSwitch.Init)
			}
			if typeSwitch.Assign != nil {
				children = append(children, typeSwitch.Assign)
			}
			if typeSwitch.Body != nil {
				children = append(children, typeSwitch.Body)
			}
			return children
		})
	registerNode(&ast.SelectStmt{}, "SelectStmt",
		func(_ *ast.SelectStmt) string { return "" },
		func(selectStmt *ast.SelectStmt) []ast.Node {
			if selectStmt.Body != nil {
				return []ast.Node{selectStmt.Body}
			}
			return nil
		})
	registerNode(&ast.CaseClause{}, "CaseClause",
		func(_ *ast.CaseClause) string { return "" },
		func(caseClause *ast.CaseClause) []ast.Node {
			return append(anySliceToNodes(caseClause.List), anySliceToNodes(caseClause.Body)...)
		})
	registerNode(&ast.CommClause{}, "CommClause",
		func(_ *ast.CommClause) string { return "" },
		func(commClause *ast.CommClause) []ast.Node {
			children := []ast.Node{}
			if commClause.Comm != nil {
				children = append(children, commClause.Comm)
			}
			return append(children, anySliceToNodes(commClause.Body)...)
		})
	registerNode(&ast.ReturnStmt{}, "ReturnStmt",
		func(_ *ast.ReturnStmt) string { return "" },
		func(retStmt *ast.ReturnStmt) []ast.Node {
			return anySliceToNodes(retStmt.Results)
		})
	registerNode(&ast.AssignStmt{}, "AssignStmt", func(assign *ast.AssignStmt) string {
		return fmt.Sprintf("Assign Op: %s", assign.Tok)
	}, func(assign *ast.AssignStmt) []ast.Node {
		return append(anySliceToNodes(assign.Lhs), anySliceToNodes(assign.Rhs)...)
	})
	registerNode(&ast.DeclStmt{}, "DeclStmt",
		func(_ *ast.DeclStmt) string { return "" },
		func(declStmt *ast.DeclStmt) []ast.Node {
			if declStmt.Decl != nil {
				return []ast.Node{declStmt.Decl}
			}
			return nil
		})
	registerNode(&ast.ExprStmt{}, "ExprStmt",
		func(_ *ast.ExprStmt) string { return "" },
		func(exprStmt *ast.ExprStmt) []ast.Node {
			return []ast.Node{exprStmt.X}
		})
	registerNode(&ast.IncDecStmt{}, "IncDecStmt", func(incDec *ast.IncDecStmt) string {
		return fmt.Sprintf("Op: %s", incDec.Tok)
	}, func(incDec *ast.IncDecStmt) []ast.Node {
		return []ast.Node{incDec.X}
	})
	registerNode(&ast.SendStmt{}, "SendStmt",
		func(_ *ast.SendStmt) string { return "Chan Send" },
		func(sendStmt *ast.SendStmt) []ast.Node {
			children := []ast.Node{}
			if sendStmt.Chan != nil {
				children = append(children, sendStmt.Chan)
			}
			if sendStmt.Value != nil {
				children = append(children, sendStmt.Value)
			}
			return children
		})
	registerNode(&ast.GoStmt{}, "GoStmt",
		func(_ *ast.GoStmt) string { return "Go Routine" },
		func(goStmt *ast.GoStmt) []ast.Node {
			return []ast.Node{goStmt.Call}
		})
	registerNode(&ast.DeferStmt{}, "DeferStmt",
		func(_ *ast.DeferStmt) string { return "Defer" },
		func(deferStmt *ast.DeferStmt) []ast.Node {
			return []ast.Node{deferStmt.Call}
		})
	registerNode(&ast.BranchStmt{}, "BranchStmt", func(branch *ast.BranchStmt) string {
		return fmt.Sprintf("Branch: %s", branch.Tok)
	}, func(branch *ast.BranchStmt) []ast.Node {
		if branch.Label != nil {
			return []ast.Node{branch.Label}
		}
		return nil
	})
	registerNode(&ast.LabeledStmt{}, "LabeledStmt", func(labelStmt *ast.LabeledStmt) string {
		return "Label: " + labelStmt.Label.Name
	}, func(labelStmt *ast.LabeledStmt) []ast.Node {
		children := []ast.Node{}
		if labelStmt.Label != nil {
			children = append(children, labelStmt.Label)
		}
		if labelStmt.Stmt != nil {
			children = append(children, labelStmt.Stmt)
		}
		return children
	})
}

//nolint:gocyclo,gocognit,cyclop,funlen
func registerExprNodes() {
	registerNode(&ast.BinaryExpr{}, "BinaryExpr", func(binExpr *ast.BinaryExpr) string {
		return fmt.Sprintf("Op: %s", binExpr.Op)
	}, func(binExpr *ast.BinaryExpr) []ast.Node {
		return []ast.Node{binExpr.X, binExpr.Y}
	})
	registerNode(&ast.UnaryExpr{}, "UnaryExpr", func(unaryExpr *ast.UnaryExpr) string {
		return fmt.Sprintf("Op: %s", unaryExpr.Op)
	}, func(unaryExpr *ast.UnaryExpr) []ast.Node {
		return []ast.Node{unaryExpr.X}
	})
	registerNode(&ast.CallExpr{}, "CallExpr", func(callExpr *ast.CallExpr) string {
		if fun, ok := callExpr.Fun.(*ast.Ident); ok {
			return "Call: " + fun.Name
		} else if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			return fmt.Sprintf("Method/Call: %s.%s", fmt.Sprintf("%v", sel.X), sel.Sel.Name)
		}
		return ""
	}, func(callExpr *ast.CallExpr) []ast.Node {
		children := []ast.Node{}
		if callExpr.Fun != nil {
			children = append(children, callExpr.Fun)
		}
		return append(children, anySliceToNodes(callExpr.Args)...)
	})
	registerNode(&ast.SelectorExpr{}, "SelectorExpr", func(selector *ast.SelectorExpr) string {
		return "Selector: " + selector.Sel.Name
	}, func(selector *ast.SelectorExpr) []ast.Node {
		children := []ast.Node{}
		if selector.X != nil {
			children = append(children, selector.X)
		}
		if selector.Sel != nil {
			children = append(children, selector.Sel)
		}
		return children
	})
	registerNode(&ast.IndexExpr{}, "IndexExpr",
		func(_ *ast.IndexExpr) string { return "Index" },
		func(indexExpr *ast.IndexExpr) []ast.Node {
			children := []ast.Node{}
			if indexExpr.X != nil {
				children = append(children, indexExpr.X)
			}
			if indexExpr.Index != nil {
				children = append(children, indexExpr.Index)
			}
			return children
		})
	registerNode(&ast.CompositeLit{}, "CompositeLit", func(compLit *ast.CompositeLit) string {
		return fmt.Sprintf("Literal (Elements: %d)", len(compLit.Elts))
	}, func(compLit *ast.CompositeLit) []ast.Node {
		children := []ast.Node{}
		if compLit.Type != nil {
			children = append(children, compLit.Type)
		}
		return append(children, anySliceToNodes(compLit.Elts)...)
	})
	registerNode(&ast.KeyValueExpr{}, "KeyValueExpr",
		func(_ *ast.KeyValueExpr) string { return "" },
		func(kvExpr *ast.KeyValueExpr) []ast.Node {
			children := []ast.Node{}
			if kvExpr.Key != nil {
				children = append(children, kvExpr.Key)
			}
			if kvExpr.Value != nil {
				children = append(children, kvExpr.Value)
			}
			return children
		})
	registerNode(&ast.FuncLit{}, "FuncLit",
		func(_ *ast.FuncLit) string { return "Anonymous Func" },
		func(funcLit *ast.FuncLit) []ast.Node {
			children := []ast.Node{}
			if funcLit.Type != nil {
				children = append(children, funcLit.Type)
			}
			if funcLit.Body != nil {
				children = append(children, funcLit.Body)
			}
			return children
		})
	registerNode(&ast.ParenExpr{}, "ParenExpr",
		func(_ *ast.ParenExpr) string { return "" },
		func(paren *ast.ParenExpr) []ast.Node {
			return []ast.Node{paren.X}
		})
	registerNode(&ast.StarExpr{}, "StarExpr",
		func(_ *ast.StarExpr) string { return "" },
		func(star *ast.StarExpr) []ast.Node {
			return []ast.Node{star.X}
		})
	registerNode(&ast.SliceExpr{}, "SliceExpr",
		func(sliceExpr *ast.SliceExpr) string {
			if sliceExpr.Slice3 {
				return "Slice [low:high:max]"
			}
			return "Slice [low:high]"
		},
		func(sliceExpr *ast.SliceExpr) []ast.Node {
			children := []ast.Node{sliceExpr.X}
			if sliceExpr.Low != nil {
				children = append(children, sliceExpr.Low)
			}
			if sliceExpr.High != nil {
				children = append(children, sliceExpr.High)
			}
			if sliceExpr.Max != nil {
				children = append(children, sliceExpr.Max)
			}
			return children
		})
	registerNode(&ast.IndexListExpr{}, "IndexListExpr",
		func(_ *ast.IndexListExpr) string { return "Generic Instantiation" },
		func(indexList *ast.IndexListExpr) []ast.Node {
			return append([]ast.Node{indexList.X}, anySliceToNodes(indexList.Indices)...)
		})
	registerNode(&ast.TypeAssertExpr{}, "TypeAssertExpr",
		func(_ *ast.TypeAssertExpr) string { return "Type Assertion" },
		func(typeAssert *ast.TypeAssertExpr) []ast.Node {
			children := []ast.Node{}
			if typeAssert.X != nil {
				children = append(children, typeAssert.X)
			}
			if typeAssert.Type != nil {
				children = append(children, typeAssert.Type)
			}
			return children
		})
	registerNode(&ast.Ellipsis{}, "Ellipsis",
		func(_ *ast.Ellipsis) string { return "Variadic ..." },
		func(ellipsis *ast.Ellipsis) []ast.Node {
			if ellipsis.Elt != nil {
				return []ast.Node{ellipsis.Elt}
			}
			return nil
		})
}

func registerTypeNodes() {
	registerNode(&ast.InterfaceType{}, "InterfaceType", func(iface *ast.InterfaceType) string {
		return fmt.Sprintf("Interface (Methods: %d)", len(iface.Methods.List))
	}, func(iface *ast.InterfaceType) []ast.Node {
		if iface.Methods != nil {
			return []ast.Node{iface.Methods}
		}
		return nil
	})
	registerNode(&ast.StructType{}, "StructType", func(structType *ast.StructType) string {
		return fmt.Sprintf("Struct (Fields: %d)", len(structType.Fields.List))
	}, func(structType *ast.StructType) []ast.Node {
		if structType.Fields != nil {
			return []ast.Node{structType.Fields}
		}
		return nil
	})
	registerNode(&ast.ArrayType{}, "ArrayType",
		func(_ *ast.ArrayType) string { return "Array/Slice" },
		func(arrayType *ast.ArrayType) []ast.Node {
			children := []ast.Node{}
			if arrayType.Len != nil {
				children = append(children, arrayType.Len)
			}
			if arrayType.Elt != nil {
				children = append(children, arrayType.Elt)
			}
			return children
		})
	registerNode(&ast.MapType{}, "MapType",
		func(_ *ast.MapType) string { return "Map" },
		func(mapType *ast.MapType) []ast.Node {
			children := []ast.Node{}
			if mapType.Key != nil {
				children = append(children, mapType.Key)
			}
			if mapType.Value != nil {
				children = append(children, mapType.Value)
			}
			return children
		})
	registerNode(&ast.ChanType{}, "ChanType", func(chanType *ast.ChanType) string {
		return fmt.Sprintf("Chan (Dir: %v)", chanType.Dir)
	}, func(chanType *ast.ChanType) []ast.Node {
		return []ast.Node{chanType.Value}
	})
	registerNode(&ast.FieldList{}, "FieldList", func(fieldList *ast.FieldList) string {
		return fmt.Sprintf("Fields: %d", fieldList.NumFields())
	}, func(fieldList *ast.FieldList) []ast.Node {
		return anySliceToNodes(fieldList.List)
	})
}

func registerMiscNodes() {
	registerNode(&ast.CommentGroup{}, "CommentGroup",
		func(commentGroup *ast.CommentGroup) string {
			text := commentGroup.List[0].Text
			const previewLen = 20
			if len(text) > previewLen {
				_ = text // lint SA4006: intentionally unused after truncation for display
			}
			return fmt.Sprintf("CommentGroup (%d lines)", len(commentGroup.List))
		},
		func(commentGroup *ast.CommentGroup) []ast.Node {
			nodes := make([]ast.Node, len(commentGroup.List))
			for i, comm := range commentGroup.List {
				nodes[i] = comm // ← now returns individual *ast.Comment
			}
			return nodes
		})
	registerNode(&ast.Ident{}, "Ident", func(ident *ast.Ident) string {
		return "Name: " + ident.Name
	}, func(_ *ast.Ident) []ast.Node { return nil })
	registerNode(&ast.BasicLit{}, "BasicLit", func(basicLit *ast.BasicLit) string {
		return fmt.Sprintf("Value: %s (Kind: %s)", basicLit.Value, basicLit.Kind)
	}, func(_ *ast.BasicLit) []ast.Node { return nil })
	registerNode(&ast.ImportSpec{}, "ImportSpec", func(importSpec *ast.ImportSpec) string {
		return fmt.Sprintf("Import: %q", importSpec.Path.Value)
	}, func(importSpec *ast.ImportSpec) []ast.Node {
		return []ast.Node{importSpec.Path}
	})
	registerNode(&ast.Comment{}, "Comment", func(comment *ast.Comment) string {
		text := comment.Text
		if len(text) > 20 {
			text = text[:20] + "..."
		}
		return "Single Comment: " + text
	}, func(_ *ast.Comment) []ast.Node { return nil })
}

func registerNode[T ast.Node](prototype T, kind string, descFn func(T) string, childFn func(T) []ast.Node) {
	nodeType := reflect.TypeOf(prototype)
	nodeKindMap[nodeType] = kind

	descFuncMap[nodeType] = func(n ast.Node) string {
		if assertVal, ok := n.(T); ok {
			return descFn(assertVal)
		}
		return ""
	}

	childFuncMap[nodeType] = func(n ast.Node) []ast.Node {
		if assertVal, ok := n.(T); ok {
			return childFn(assertVal)
		}
		return nil
	}
}

// PrintTree renders an AST node tree structure to a writer, displaying node types, positions, and relationships.
// It respects the PrintEnabled flag and returns nil if PrintEnabled is false or node is nil.
func PrintTree(writer io.Writer, fset *token.FileSet, node ast.Node) error {
	if !PrintEnabled || node == nil {
		return nil
	}

	var strBuilder strings.Builder

	type stackEntry struct {
		node   ast.Node
		prefix string
		isLast bool
	}

	stack := []stackEntry{{node: node, prefix: "", isLast: true}}

	for len(stack) > 0 {
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		connector := "└── "
		if !entry.isLast {
			connector = "├── "
		}

		processNode(&strBuilder, fset, entry.node, entry.prefix, connector)

		nodeType := reflect.TypeOf(entry.node)
		var children []ast.Node
		if childFn, ok := childFuncMap[nodeType]; ok && childFn != nil {
			children = childFn(entry.node)
		}

		nextPrefix := entry.prefix + "    "
		if !entry.isLast {
			nextPrefix = entry.prefix + "│   "
		}

		for i := len(children) - 1; i >= 0; i-- { // Reverse for correct order
			stack = append(stack, stackEntry{node: children[i], prefix: nextPrefix, isLast: i == 0})
		}
	}

	if _, err := writer.Write([]byte(strBuilder.String())); err != nil {
		return fmt.Errorf("failed to write AST tree: %w", err)
	}
	return nil
}

func processNode(strBuilder *strings.Builder, fset *token.FileSet, node ast.Node, prefix string, connector string) {
	pos := fset.Position(node.Pos())
	nodeType := reflect.TypeOf(node)
	kind, ok := nodeKindMap[nodeType]
	if !ok {
		kindStr := fmt.Sprintf("%T", node)
		if len(kindStr) > len(astPrefix) && strings.HasPrefix(kindStr, astPrefix) {
			kind = kindStr[len(astPrefix):]
		}
	}
	loc := fmt.Sprintf(" [%d:%d]", pos.Line, pos.Column)

	fmt.Fprintf(strBuilder, "%s%s%s%s\n", prefix, connector, kind, loc)

	var desc string
	if descFn, ok := descFuncMap[nodeType]; ok && descFn != nil {
		desc = descFn(node)
	}
	if desc != "" {
		fmt.Fprintf(strBuilder, "%s    └── %s\n", prefix, desc)
	}
}

// optimized with cap
func anySliceToNodes[T ast.Node](s []T) []ast.Node {
	result := make([]ast.Node, 0, len(s))
	for _, v := range s {
		result = append(result, v)
	}
	return result
}
