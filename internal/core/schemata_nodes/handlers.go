package schemata_nodes

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/analysis"
)

type SchemataHandler func(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node

type MutantForSite struct {
	ID            int
	Op            mutator.Operator
	ReturnType    string
	EnclosingFunc *ast.FuncDecl
}

type mutantResult struct {
	id      int
	retStmt *ast.ReturnStmt
}

type NodeType uint8

const (
	NTUnknown NodeType = iota
	NTBinaryExpr
	NTUnaryExpr
	NTCallExpr
	NTIdent
	NTBasicLit
	NTCaseClause
	NTIfStmt
	NTForStmt
	NTRangeStmt
	NTAssignStmt
	NTIncDecStmt
	NTDeferStmt
	NTGoStmt
	NTSendStmt
	NTSwitchStmt
	NTTypeSwitchStmt
	NTReturnStmt
	NTBranchStmt
	NTSelectStmt
	NTCommClause
	NTLabeledStmt
	NTExprStmt
	NTDeclStmt
	NTErrStmt
	NTBlockStmt
	NTFuncDecl
	NTMax
)

var handlers = make([]SchemataHandler, NTMax)

func init() {
	exprHandler := makeExprHandler()
	stmtHandler := makeStmtHandler()

	
	handlers[NTBinaryExpr] = exprHandler
	handlers[NTUnaryExpr] = exprHandler
	handlers[NTCallExpr] = exprHandler
	handlers[NTIdent] = exprHandler
	handlers[NTBasicLit] = exprHandler

	
	handlers[NTIfStmt] = stmtHandler
	handlers[NTForStmt] = stmtHandler
	handlers[NTBlockStmt] = stmtHandler
	handlers[NTBranchStmt] = stmtHandler
	handlers[NTIncDecStmt] = stmtHandler
	handlers[NTGoStmt] = stmtHandler
	handlers[NTSendStmt] = stmtHandler
	handlers[NTSwitchStmt] = stmtHandler
	handlers[NTTypeSwitchStmt] = stmtHandler
	handlers[NTSelectStmt] = stmtHandler
	handlers[NTCommClause] = stmtHandler
	handlers[NTLabeledStmt] = stmtHandler
	handlers[NTExprStmt] = stmtHandler
	handlers[NTDeclStmt] = stmtHandler
	handlers[NTErrStmt] = stmtHandler
	handlers[NTFuncDecl] = stmtHandler

	
	handlers[NTCaseClause] = HandleCaseClause
	handlers[NTRangeStmt] = HandleRangeStmt
	handlers[NTAssignStmt] = HandleAssignStmt
	handlers[NTDeferStmt] = HandleDeferStmt
	handlers[NTReturnStmt] = HandleReturnStmt
}

func GetHandler(node ast.Node) SchemataHandler {
	return handlers[NodeTypeOf(node)]
}

func NodeTypeOf(node ast.Node) NodeType {
	switch node.(type) {
	case *ast.BinaryExpr:
		return NTBinaryExpr
	case *ast.UnaryExpr:
		return NTUnaryExpr
	case *ast.CallExpr:
		return NTCallExpr
	case *ast.Ident:
		return NTIdent
	case *ast.BasicLit:
		return NTBasicLit
	case *ast.CaseClause:
		return NTCaseClause
	case *ast.IfStmt:
		return NTIfStmt
	case *ast.ForStmt:
		return NTForStmt
	case *ast.RangeStmt:
		return NTRangeStmt
	case *ast.AssignStmt:
		return NTAssignStmt
	case *ast.IncDecStmt:
		return NTIncDecStmt
	case *ast.DeferStmt:
		return NTDeferStmt
	case *ast.GoStmt:
		return NTGoStmt
	case *ast.SendStmt:
		return NTSendStmt
	case *ast.SwitchStmt:
		return NTSwitchStmt
	case *ast.TypeSwitchStmt:
		return NTTypeSwitchStmt
	case *ast.ReturnStmt:
		return NTReturnStmt
	case *ast.BranchStmt:
		return NTBranchStmt
	case *ast.SelectStmt:
		return NTSelectStmt
	case *ast.CommClause:
		return NTCommClause
	case *ast.LabeledStmt:
		return NTLabeledStmt
	case *ast.ExprStmt:
		return NTExprStmt
	case *ast.DeclStmt:
		return NTDeclStmt
	case *ast.EmptyStmt:
		return NTErrStmt
	case *ast.BlockStmt:
		return NTBlockStmt
	case *ast.FuncDecl:
		return NTFuncDecl
	default:
		return NTUnknown
	}
}

func (nt NodeType) ToUint8() uint8 {
	return uint8(nt)
}

func NodeTypeToUint8(node ast.Node) uint8 {
	return NodeTypeOf(node).ToUint8()
}



func makeExprHandler() SchemataHandler {
	return func(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
		if len(mutants) == 0 {
			return original
		}
		if len(mutants) == 1 {
			return applySingleMutant(original, mutants[0], returnType, file)
		}
		return wrapExpressionMulti(original, mutants, returnType, file)
	}
}

func makeStmtHandler() SchemataHandler {
	return func(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
		if len(mutants) == 0 {
			return original
		}
		if len(mutants) == 1 {
			return applySingleMutant(original, mutants[0], returnType, file)
		}
		return WrapStatement(original, mutants, returnType, file)
	}
}



func applySingleMutant(original ast.Node, mutant MutantForSite, returnType string, file *ast.File) ast.Node {
	mutated := mutator.ApplyOperator(mutant.Op, original, returnType, file, mutant.EnclosingFunc)
	if mutated == nil {
		return original
	}
	// Build a type map from the enclosing function so wrapExprWithIf can infer
	// the expression's actual type (not just the function's return type).
	var typeMap map[string]string
	if mutant.EnclosingFunc != nil {
		typeMap = analysis.BuildTypeMap(mutant.EnclosingFunc)
	}
	return wrapWithSchemata(original, mutated, mutant.ID, returnType, typeMap)
}

func wrapWithSchemata(original, mutated ast.Node, mutantID int, returnType string, typeMap map[string]string) ast.Node {
	switch orig := original.(type) {
	case ast.Expr:
		mutExpr, ok := mutated.(ast.Expr)
		if !ok {
			return original
		}
		return wrapExprWithIf(mutExpr, orig, mutantID, returnType, typeMap)

	case ast.Stmt:
		mutStmt, ok := mutated.(ast.Stmt)
		if !ok {
			return original
		}
		return &ast.IfStmt{
			Cond: createMutantIDCondition(mutantID),
			Body: &ast.BlockStmt{List: []ast.Stmt{mutStmt}},
			Else: &ast.BlockStmt{List: []ast.Stmt{orig}},
		}

	default:
		return mutated
	}
}

func wrapExprWithIf(mutated, original ast.Expr, mutantID int, returnType string, typeMap map[string]string) ast.Expr {
	// Use the expression's own type, not the enclosing function's return type.
	// Example: `n > min` is always `bool` even inside a func returning `int`.
	// Using the wrong type produces closures like `func() int { return n >= min }`
	// where `n >= min` is bool — a compile error in every branch.
	exprType := inferExprType(original, returnType, typeMap)
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: safeFuncType(&ast.FieldList{
				List: []*ast.Field{{Type: buildTypeExpr(exprType)}},
			}),
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.IfStmt{
						Cond: createMutantIDCondition(mutantID),
						Body: &ast.BlockStmt{
							List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{mutated}}},
						},
					},
					&ast.ReturnStmt{Results: []ast.Expr{original}},
				},
			},
		},
	}
}



func wrapExpressionMulti(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	origExpr := original.(ast.Expr)

	// Build typeMap from the first mutant's enclosing function so we can infer
	// the expression's actual type (not the function's return type).
	var typeMap map[string]string
	if len(mutants) > 0 && mutants[0].EnclosingFunc != nil {
		typeMap = analysis.BuildTypeMap(mutants[0].EnclosingFunc)
	}
	exprType := inferExprType(origExpr, returnType, typeMap)

	stmts := make([]ast.Stmt, 0, len(mutants)+1)

	for _, m := range mutants {
		mutated := mutator.ApplyOperator(m.Op, original, returnType, file, m.EnclosingFunc)
		if mutated == nil {
			continue
		}
		mutExpr, ok := mutated.(ast.Expr)
		if !ok {
			continue
		}
		stmts = append(stmts, &ast.IfStmt{
			Cond: createMutantIDCondition(m.ID),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{mutExpr}}}},
		})
	}

	if len(stmts) == 0 {
		return original
	}

	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{origExpr}})

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: safeFuncType(&ast.FieldList{
				List: []*ast.Field{{Type: buildTypeExpr(exprType)}},
			}),
			Body: &ast.BlockStmt{List: stmts},
		},
	}
}



func HandleCaseClause(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	cc, ok := original.(*ast.CaseClause)
	if !ok {
		return original
	}
	if len(mutants) != 1 {
		return cc
	}

	if returnType == "" {
		returnType = "string"
	}

	zeroVal := GetZeroValueForType(returnType)

	newBody := make([]ast.Stmt, 0, len(cc.Body)+len(mutants))

	for _, mutant := range mutants {
		mutated := mutator.ApplyOperator(mutant.Op, cc, returnType, file, nil)
		if mutated == nil {
			continue
		}

		mutatedCC, ok := mutated.(*ast.CaseClause)
		if !ok {
			continue
		}

		if len(mutatedCC.Body) == 0 {
			newBody = append(newBody, &ast.IfStmt{
				Cond: createMutantIDCondition(mutant.ID),
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{Results: []ast.Expr{zeroVal}},
					},
				},
			})
		} else {
			newBody = append(newBody, &ast.IfStmt{
				Cond: createMutantIDCondition(mutant.ID),
				Body: &ast.BlockStmt{
					List: mutatedCC.Body,
				},
			})
		}
	}

	newBody = append(newBody, cc.Body...)

	return &ast.CaseClause{
		Case:  cc.Case,
		List:  cc.List,
		Colon: cc.Colon,
		Body:  newBody,
	}
}

func HandleRangeStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 1 {
		return applySingleMutant(original, mutants[0], returnType, file)
	}
	return WrapStatement(original, mutants, returnType, file)
}

func HandleAssignStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
    if assign, ok := original.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
        // Never wrap :=  — it would pull the declared variable out of scope.
        // Expression-level operators will still mutate the RHS individually.
        return original
    }
    if len(mutants) == 1 {
        return applySingleMutant(original, mutants[0], returnType, file)
    }
    return WrapStatement(original, mutants, returnType, file)
}

func HandleDeferStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 1 {
		return applySingleMutant(original, mutants[0], returnType, file)
	}
	return WrapStatement(original, mutants, returnType, file)
}

func HandleReturnStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	originalRet, ok := original.(*ast.ReturnStmt)
	if !ok {
		return original
	}
	if len(originalRet.Results) == 0 {
		return original
	}

	// Parse multi-value return types
	returnTypes := parseReturnTypes(returnType)
	numReturns := len(returnTypes)
	
	// For multi-value returns, only wrap if we're mutating the entire return statement
	// Don't wrap individual expressions within multi-value returns
	if numReturns > 1 && len(originalRet.Results) != numReturns {
		// Mismatch - skip wrapping to avoid compilation errors
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		mutated := mutator.ApplyOperator(mutant.Op, original, returnType, file, mutant.EnclosingFunc)
		if mutated == nil {
			return original
		}
		if retStmt, ok := mutated.(*ast.ReturnStmt); ok && len(retStmt.Results) > 0 {
			// Check if mutated statement has multiple return values
			if len(retStmt.Results) > 1 {
				// Multi-value return - wrap the entire statement
				return wrapMultiReturnWithSchemata(originalRet, retStmt, mutant.ID, returnTypes)
			}
			// Single-value return - wrap just the expression
			expr := retStmt.Results[0]
			if isNilExpr(expr) && !isNilableType(returnTypes[0]) {
				return original
			}
			return wrapReturnWithSchemata(originalRet, expr, mutant.ID, returnTypes[0])
		}
		return mutated
	}

	// Multi-mutant path
	var mutResults []mutantResult

	for _, mutant := range mutants {
		mutReturnType := mutant.ReturnType
		if mutReturnType == "" {
			mutReturnType = returnType
		}
		mutated := mutator.ApplyOperator(mutant.Op, original, mutReturnType, file, mutant.EnclosingFunc)
		if mutated == nil {
			continue
		}

		mutatedRet, ok := mutated.(*ast.ReturnStmt)
		if !ok || len(mutatedRet.Results) == 0 {
			continue
		}
		
		// Skip mutations that don't match the expected return count
		if len(mutatedRet.Results) != len(returnTypes) {
			continue
		}

		// Skip type-unsafe nil mutations
		if numReturns == 1 {
			expr := mutatedRet.Results[0]
			if isNilExpr(expr) && !isNilableType(returnTypes[0]) {
				continue
			}
		}

		mutResults = append(mutResults, mutantResult{id: mutant.ID, retStmt: mutatedRet})
	}

	if len(mutResults) == 0 {
		return original
	}

	if len(mutResults) == 1 {
		mr := mutResults[0]
		if len(mr.retStmt.Results) > 1 {
			// Multi-value return
			return wrapMultiReturnWithSchemata(originalRet, mr.retStmt, mr.id, returnTypes)
		}
		// Single-value return
		return wrapReturnWithSchemata(originalRet, mr.retStmt.Results[0], mr.id, returnTypes[0])
	}

	// Multiple mutants - check if we have multi-value returns
	hasMultiValue := len(originalRet.Results) > 1
	if hasMultiValue {
		return wrapMultiReturnMultiMutants(originalRet, mutResults, returnTypes)
	}

	// Single-value return with multiple mutants
	origExpr := originalRet.Results[0]
	typeExpr := buildTypeExpr(returnTypes[0])

	stmts := make([]ast.Stmt, 0, len(mutResults)+1)
	for _, mr := range mutResults {
		stmts = append(stmts, &ast.IfStmt{
			Cond: createMutantIDCondition(mr.id),
			Body: &ast.BlockStmt{
				List: []ast.Stmt{&ast.ReturnStmt{Results: mr.retStmt.Results}},
			},
		})
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{origExpr}})

	return &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: safeFuncType(&ast.FieldList{
						List: []*ast.Field{{Type: typeExpr}},
					}),
					Body: &ast.BlockStmt{List: stmts},
				},
			},
		},
	}
}

// parseReturnTypes splits comma-separated return types
func parseReturnTypes(returnType string) []string {
	if returnType == "" {
		return []string{"interface{}"}
	}
	if !strings.Contains(returnType, ",") {
		return []string{returnType}
	}
	types := strings.Split(returnType, ",")
	for i := range types {
		types[i] = strings.TrimSpace(types[i])
	}
	return types
}

// wrapMultiReturnWithSchemata wraps a multi-value return statement with schemata
func wrapMultiReturnWithSchemata(original, mutated *ast.ReturnStmt, mutantID int, returnTypes []string) ast.Node {
	// Build field list for function type
	fields := make([]*ast.Field, len(returnTypes))
	for i, typ := range returnTypes {
		fields[i] = &ast.Field{Type: buildTypeExpr(typ)}
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: safeFuncType(&ast.FieldList{List: fields}),
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.IfStmt{
								Cond: createMutantIDCondition(mutantID),
								Body: &ast.BlockStmt{
									List: []ast.Stmt{&ast.ReturnStmt{Results: mutated.Results}},
								},
							},
							&ast.ReturnStmt{Results: original.Results},
						},
					},
				},
			},
		},
	}
}

// wrapMultiReturnMultiMutants wraps multi-value returns with multiple mutants
func wrapMultiReturnMultiMutants(original *ast.ReturnStmt, mutResults []mutantResult, returnTypes []string) ast.Node {
	// Build field list for function type
	fields := make([]*ast.Field, len(returnTypes))
	for i, typ := range returnTypes {
		fields[i] = &ast.Field{Type: buildTypeExpr(typ)}
	}

	stmts := make([]ast.Stmt, 0, len(mutResults)+1)
	for _, mr := range mutResults {
		stmts = append(stmts, &ast.IfStmt{
			Cond: createMutantIDCondition(mr.id),
			Body: &ast.BlockStmt{
				List: []ast.Stmt{&ast.ReturnStmt{Results: mr.retStmt.Results}},
			},
		})
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: original.Results})

	return &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: safeFuncType(&ast.FieldList{List: fields}),
					Body: &ast.BlockStmt{List: stmts},
				},
			},
		},
	}
}

func wrapReturnWithSchemata(original *ast.ReturnStmt, mutatedExpr ast.Expr, mutantID int, returnType string) ast.Node {
	// Always use the declared function return type for the closure.
	// Do NOT narrow the type (e.g. from interface{} to string) based on what the
	// expression looks like — nil is valid in interface{} but not in string/int.
	typeExpr := buildTypeExpr(returnType)
	if returnType == "" {
		// Fallback only when there is genuinely no return type info.
		typeExpr = &ast.Ident{Name: "interface{}"}
	}
	return &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: safeFuncType(&ast.FieldList{
						List: []*ast.Field{{Type: typeExpr}},
					}),
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.IfStmt{
								Cond: createMutantIDCondition(mutantID),
								Body: &ast.BlockStmt{
									List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{mutatedExpr}}},
								},
							},
							&ast.ReturnStmt{Results: original.Results},
						},
					},
				},
			},
		},
	}
}



func buildTypeExpr(resultType string) ast.Expr {
	if resultType == "" {
		return &ast.Ident{Name: "interface{}"}
	}
	if strings.HasPrefix(resultType, "*") {
		baseType := strings.TrimPrefix(resultType, "*")
		if baseType != "" {
			return &ast.StarExpr{X: &ast.Ident{Name: baseType}}
		}
	}
	return &ast.Ident{Name: resultType}
}

var mutantIDCondCache [256]*ast.BinaryExpr

func createMutantIDCondition(mutantID int) *ast.BinaryExpr {
	if mutantID >= 0 && mutantID < len(mutantIDCondCache) {
		if cond := mutantIDCondCache[mutantID]; cond != nil {
			return cond
		}
		cond := &ast.BinaryExpr{
			X:  &ast.Ident{Name: "activeMutantID"},
			Op: token.EQL,
			Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutantID)},
		}
		mutantIDCondCache[mutantID] = cond
		return cond
	}
	return &ast.BinaryExpr{
		X:  &ast.Ident{Name: "activeMutantID"},
		Op: token.EQL,
		Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutantID)},
	}
}

func GetZeroValueForType(returnType string) ast.Expr {
	switch returnType {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return &ast.BasicLit{Kind: token.INT, Value: "0"}
	case "float32", "float64":
		return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}
	case "string":
		return &ast.BasicLit{Kind: token.STRING, Value: "\"\""}
	case "bool":
		return &ast.Ident{Name: "false"}
	default:
		return &ast.Ident{Name: "nil"}
	}
}

func buildIfElseChain(originalStmt ast.Stmt, mutants []MutantForSite, file *ast.File,
	extractMutated func(mutant MutantForSite) (ast.Stmt, bool)) ast.Node {

	type mutPair struct {
		id   int
		stmt ast.Stmt
	}
	var pairs []mutPair
	for _, mutant := range mutants {
		mutatedStmt, ok := extractMutated(mutant)
		if !ok {
			continue
		}
		pairs = append(pairs, mutPair{mutant.ID, mutatedStmt})
	}
	if len(pairs) == 0 {
		return originalStmt
	}

	var chain ast.Stmt = &ast.BlockStmt{List: []ast.Stmt{originalStmt}}
	for i := len(pairs) - 1; i >= 0; i-- {
		chain = &ast.IfStmt{
			Cond: createMutantIDCondition(pairs[i].id),
			Body: &ast.BlockStmt{List: []ast.Stmt{pairs[i].stmt}},
			Else: chain,
		}
	}

	return &ast.BlockStmt{List: []ast.Stmt{chain}}
}

func WrapStatement(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	originalStmt, ok := original.(ast.Stmt)
	if !ok {
		return original
	}

	return buildIfElseChain(originalStmt, mutants, file, func(mutant MutantForSite) (ast.Stmt, bool) {
		mutated := mutator.ApplyOperator(mutant.Op, original, returnType, file, nil)
		if mutated == nil {
			return nil, false
		}
		mutatedStmt, ok := mutated.(ast.Stmt)
		return mutatedStmt, ok
	})
}

func safeFuncType(results *ast.FieldList) *ast.FuncType {
    return &ast.FuncType{
        Params:  &ast.FieldList{}, // must not be nil
        Results: results,
    }
}

func isConcreteFallbackType(t string) bool {
	if t == "" || t == "interface{}" {
		return false
	}
	// Interface literal syntax: interface{ ... } or struct{ ... }
	if strings.Contains(t, "{") {
		return false
	}
	// Package-qualified types (ast.Node, io.Reader, etc.) may be interfaces.
	// We can't distinguish struct vs interface at this syntactic level, so
	// be conservative and exclude them from the fallback.
	if strings.Contains(t, ".") {
		return false
	}
	return true
}

func inferExprType(expr ast.Expr, siteReturnType string, typeMap map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if typeMap != nil {
			if typ, ok := typeMap[e.Name]; ok {
				if strings.Contains(typ, ".") {
					// Package-qualified types (ast.Node, io.Reader, etc.) may be
					// interfaces. Use them only when context exactly matches —
					// otherwise an ast.Node-typed IIFE ends up in a bool position
					// and slips past the lenient importer in L3.
					if typ == siteReturnType {
						return typ
					}
					// Fall through to keyword/heuristic checks below
				} else {
					return typ
				}
			}
		}
		switch e.Name {
		case "true", "false":
			return "bool"
		case "nil":
			return "interface{}"
		case "int", "int8", "int16", "int32", "int64":
			return "int"
		case "uint", "uint8", "uint16", "uint32", "uint64":
			return "uint"
		case "float32", "float64":
			return "float64"
		case "string":
			return "string"
		case "bool":
			return "bool"
		case "byte":
			if siteReturnType != "" && siteReturnType != "interface{}" {
				return siteReturnType
			}
			return "byte"
		case "rune":
			if siteReturnType != "" && siteReturnType != "interface{}" {
				return siteReturnType
			}
			return "rune"
		default:
			// Use siteReturnType as fallback only when it is a concrete,
			// unqualified type (int, float64, string, bool, *Foo, []T, etc.).
			// Interface types like ast.Node contain a "." and must NOT be used
			// here — doing so would wrap a bool expression in func() ast.Node{}
			// which breaks type checking at the call site.
			if isConcreteFallbackType(siteReturnType) {
				return siteReturnType
			}
			return "interface{}"
		}
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			if siteReturnType == "byte" {
				return "byte"
			}
			return "rune"
		}
	case *ast.BinaryExpr:
		if isComparisonOp(e.Op) || isLogicalOp(e.Op) {
			return "bool"
		}
		// For arithmetic operators, propagate the concrete type from either
		// operand. This fixes cases like `sum + 1` where `sum` is unknown but
		// the literal `1` tells us the type is int, or where siteReturnType
		// propagates correctly through one of the operands.
		lType := inferExprType(e.X, siteReturnType, typeMap)
		if lType != "" && lType != "interface{}" {
			return lType
		}
		rType := inferExprType(e.Y, siteReturnType, typeMap)
		if rType != "" && rType != "interface{}" {
			return rType
		}
		return "interface{}"
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return "bool"
		}
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.CallExpr:
		// If this is a schemata-generated IIFE (Fun is a FuncLit with a single
		// return type), extract that type. This fixes nested-mutation scenarios:
		// when inner expressions are already wrapped, the outer mutation must
		// see through the wrapper to get the real type, not fall back to
		// interface{}.
		if fl, ok := e.Fun.(*ast.FuncLit); ok &&
			fl.Type != nil &&
			fl.Type.Results != nil &&
			len(fl.Type.Results.List) == 1 {
			if t := typeToString(fl.Type.Results.List[0].Type); t != "" {
				return t
			}
		}
		// Infer well-known builtin return types so IIFEs don't inherit the
		// enclosing function's return type when used as sub-expressions.
		if fn, ok := e.Fun.(*ast.Ident); ok {
			switch fn.Name {
			case "len", "cap":
				return "int"
			case "real", "imag":
				return "float64"
			case "new":
				return "interface{}"
			}
		}
		// For all other calls: do NOT fall back to siteReturnType.
		// The call expression's own return type is independent of the
		// enclosing function's return type. Using siteReturnType here was
		// the source of "cannot use func() ast.Node{}() as bool" errors.
		return "interface{}"
	case *ast.StarExpr:
		return "*" + inferExprType(e.X, siteReturnType, typeMap)
	case *ast.IndexExpr:
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.SliceExpr:
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.SelectorExpr:
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.TypeAssertExpr:
		if e.Type != nil {
			return typeToString(e.Type)
		}
		return "interface{}"
	case *ast.CompositeLit:
		return typeToString(e.Type)
	case *ast.FuncLit:
		return "func"
	case *ast.ParenExpr:
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.KeyValueExpr:
		return inferExprType(e.Value, siteReturnType, typeMap)
	}
	return "interface{}"
}


func isComparisonOp(op token.Token) bool {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	}
	return false
}

func isLogicalOp(op token.Token) bool {
	switch op {
	case token.LAND, token.LOR:
		return true
	}
	return false
}

func typeToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + typeToString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + typeToString(e.Elt)
		}
		return "[" + formatNode(e.Len) + "]" + typeToString(e.Elt)
	case *ast.MapType:
		return "map[" + typeToString(e.Key) + "]" + typeToString(e.Value)
	case *ast.ChanType:
		return "chan " + typeToString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	default:
		return ""
	}
}

func formatNode(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
}

// isNilExpr reports whether expr is the nil identifier.
func isNilExpr(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "nil"
	}
	return false
}

// isNilableType reports whether a Go type can legally hold nil.
// Nilable: pointers (*T), slices ([]T), maps (map[K]V), channels (chan T),
// interfaces (interface{}), and function types (func ...).
// Non-nilable: bool, int, float, string, byte, rune, and named numeric types.
func isNilableType(t string) bool {
	if t == "" {
		return true // unknown — assume nilable to be safe
	}
	switch t {
	case "bool",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64",
		"complex64", "complex128",
		"string", "byte", "rune":
		return false
	}
	// Pointers, slices, maps, channels, interfaces, funcs are all nilable.
	if strings.HasPrefix(t, "*") ||
		strings.HasPrefix(t, "[]") ||
		strings.HasPrefix(t, "map[") ||
		strings.HasPrefix(t, "chan ") ||
		strings.HasPrefix(t, "func") ||
		t == "interface{}" {
		return true
	}
	// Named/custom types — assume nilable to avoid false positives.
	return true
}
