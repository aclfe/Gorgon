package schemata_nodes

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/analysis"
)

type MutantForSite struct {
	ID            int
	Op            mutator.Operator
	ReturnType    string
	EnclosingFunc *ast.FuncDecl
	ExprType      string
	Hint          string
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

type Variant struct {
	ID       int
	Mutated  ast.Node
	CondKind mutator.ConditionKind
	LoopKind mutator.LoopMutation
}

type SitePlan struct {
	Variants   []Variant
	ExprType   string
	NotApplied map[int]string
}

func BuildPlan(node ast.Node, mutants []MutantForSite, parent ast.Node, file *ast.File, enclosing *ast.FuncDecl) SitePlan {
	plan := SitePlan{NotApplied: make(map[int]string)}
	for _, m := range mutants {
		if plan.ExprType == "" && m.ExprType != "" {
			plan.ExprType = m.ExprType
		}
	}
	if expr, ok := node.(ast.Expr); ok && plan.ExprType == "" {
		retType := ""
		if len(mutants) > 0 {
			retType = mutants[0].ReturnType
		}
		var typeMap map[string]string
		if enclosing != nil {
			typeMap = analysis.BuildTypeMap(enclosing)
		}
		if _, isLit := expr.(*ast.BasicLit); isLit {
			if be, ok := parent.(*ast.BinaryExpr); ok && be.Op != token.SHL && be.Op != token.SHR {
				other := be.X
				if other == expr {
					other = be.Y
				}
				if _, otherLit := other.(*ast.BasicLit); !otherLit {
					plan.ExprType = inferExprType(other, retType, typeMap)
				}
			}
		}
		if plan.ExprType == "" {
			plan.ExprType = inferExprType(expr, retType, typeMap)
		}
	}

	for _, m := range mutants {
		v := Variant{ID: m.ID}
		if cop, ok := m.Op.(mutator.ConditionOperator); ok {
			v.CondKind = cop.ConditionKind()
		}
		if lop, ok := m.Op.(mutator.LoopOperator); ok {
			v.LoopKind = lop.LoopMutation()
		}
		ctx := mutator.Context{
			ReturnType:    m.ReturnType,
			File:          file,
			EnclosingFunc: enclosing,
			Parent:        parent,
			Hint:          m.Hint,
		}
		v.Mutated = mutator.ApplyOperatorWithContext(m.Op, node, ctx)
		if v.Mutated == nil {
			plan.NotApplied[m.ID] = fmt.Sprintf("operator %s produced no mutation at this site", m.Op.Name())
			continue
		}
		plan.Variants = append(plan.Variants, v)
	}
	return plan
}

func Inject(current ast.Node, plan SitePlan) (ast.Node, []int, map[int]string) {
	notApplied := make(map[int]string, len(plan.NotApplied))
	for id, r := range plan.NotApplied {
		notApplied[id] = r
	}
	if len(plan.Variants) == 0 {
		return current, nil, notApplied
	}

	switch n := current.(type) {
	case *ast.IfStmt:
		anchor := n.Cond.Pos()
		repl, ids, na := injectCondStmt(n, &n.Cond, plan.Variants, notApplied)
		FillPositions(n.Cond, anchor)
		if repl != ast.Node(n) {
			FillPositions(repl, n.Pos())
		}
		return repl, ids, na
	case *ast.ForStmt:
		var rest []Variant
		var applied []int
		var condVariants []Variant
		for _, v := range plan.Variants {
			if v.CondKind != mutator.ConditionNone {
				condVariants = append(condVariants, v)
			} else {
				rest = append(rest, v)
			}
		}
		if len(condVariants) > 0 {
			if n.Cond == nil {
				for _, v := range condVariants {
					notApplied[v.ID] = "for loop has no condition"
				}
			} else {
				anchor := n.Cond.Pos()
				_, ids, _ := injectCondStmt(n, &n.Cond, condVariants, notApplied)
				FillPositions(n.Cond, anchor)
				applied = append(applied, ids...)
			}
		}
		ids := injectLoop(n.Body, rest, notApplied)
		applied = append(applied, ids...)
		return n, applied, notApplied
	case *ast.RangeStmt:
		ids := injectLoop(n.Body, plan.Variants, notApplied)
		return n, ids, notApplied
	case *ast.CaseClause:
		ids := injectCaseClause(n, plan.Variants, notApplied)
		return n, ids, notApplied
	case *ast.FuncDecl:
		ids := injectFuncDecl(n, plan.Variants, notApplied)
		return n, ids, notApplied
	case ast.Stmt:
		repl, ids, na := injectStmt(n, plan.Variants, notApplied)
		FillPositions(repl, n.Pos())
		return repl, ids, na
	case ast.Expr:
		repl, ids, na := injectExpr(n, plan.Variants, plan.ExprType, notApplied)
		FillPositions(repl, n.Pos())
		return repl, ids, na
	}
	for _, v := range plan.Variants {
		notApplied[v.ID] = fmt.Sprintf("unsupported site node %T", current)
	}
	return current, nil, notApplied
}

func injectCondStmt(stmt ast.Stmt, cond *ast.Expr, variants []Variant, notApplied map[int]string) (ast.Node, []int, map[int]string) {
	var applied []int
	var other []Variant
	c := *cond
	for _, v := range variants {
		switch v.CondKind {
		case mutator.ConditionForceTrue:
			c = &ast.BinaryExpr{X: idEquals(v.ID), Op: token.LOR, Y: paren(c)}
		case mutator.ConditionForceFalse:
			c = &ast.BinaryExpr{X: idNotEquals(v.ID), Op: token.LAND, Y: paren(c)}
		case mutator.ConditionNegate:
			c = &ast.BinaryExpr{X: paren(idEquals(v.ID)), Op: token.NEQ, Y: paren(c)}
		default:
			other = append(other, v)
			continue
		}
		applied = append(applied, v.ID)
	}
	*cond = c
	if len(other) == 0 {
		return stmt, applied, notApplied
	}
	repl, ids, _ := injectStmt(stmt, other, notApplied)
	return repl, append(applied, ids...), notApplied
}

func injectLoop(body *ast.BlockStmt, variants []Variant, notApplied map[int]string) []int {
	var applied []int
	if body == nil {
		for _, v := range variants {
			notApplied[v.ID] = "loop has no body"
		}
		return nil
	}
	var head, tail []ast.Stmt
	for _, v := range variants {
		switch v.LoopKind {
		case mutator.LoopSkipBody:
			g := guardIf(v.ID, &ast.BranchStmt{Tok: token.CONTINUE})
			FillPositions(g, body.Lbrace)
			head = append(head, g)
		case mutator.LoopBreakAfterBody:
			g := guardIf(v.ID, &ast.BranchStmt{Tok: token.BREAK})
			FillPositions(g, body.Rbrace)
			tail = append(tail, g)
		default:
			notApplied[v.ID] = "loop mutation not expressible as a body guard"
			continue
		}
		applied = append(applied, v.ID)
	}
	if len(head)+len(tail) == 0 {
		return applied
	}
	list := make([]ast.Stmt, 0, len(head)+len(body.List)+len(tail))
	list = append(list, head...)
	list = append(list, body.List...)
	list = append(list, tail...)
	body.List = list
	return applied
}

func injectCaseClause(cc *ast.CaseClause, variants []Variant, notApplied map[int]string) []int {
	origBody, origFall := splitFallthrough(cc.Body)
	type pair struct {
		id   int
		body []ast.Stmt
	}
	var pairs []pair
	for _, v := range variants {
		mcc, ok := v.Mutated.(*ast.CaseClause)
		if !ok {
			notApplied[v.ID] = fmt.Sprintf("case clause mutation produced %T", v.Mutated)
			continue
		}
		mBody, mFall := splitFallthrough(mcc.Body)
		if mFall != origFall {
			notApplied[v.ID] = "mutation changes a trailing fallthrough"
			continue
		}
		pairs = append(pairs, pair{v.ID, mBody})
	}
	if len(pairs) == 0 {
		return nil
	}
	var chain ast.Stmt = &ast.BlockStmt{List: origBody}
	for i := len(pairs) - 1; i >= 0; i-- {
		chain = &ast.IfStmt{
			Cond: idEquals(pairs[i].id),
			Body: &ast.BlockStmt{List: pairs[i].body},
			Else: chain,
		}
	}
	FillPositions(chain, cc.Colon)
	newBody := []ast.Stmt{chain}
	if origFall != nil {
		newBody = append(newBody, origFall)
	}
	cc.Body = newBody
	applied := make([]int, len(pairs))
	for i, p := range pairs {
		applied[i] = p.id
	}
	return applied
}

func splitFallthrough(body []ast.Stmt) ([]ast.Stmt, *ast.BranchStmt) {
	if n := len(body); n > 0 {
		if br, ok := body[n-1].(*ast.BranchStmt); ok && br.Tok == token.FALLTHROUGH {
			return body[:n-1], br
		}
	}
	return body, nil
}

func injectFuncDecl(fn *ast.FuncDecl, variants []Variant, notApplied map[int]string) []int {
	if fn.Body == nil {
		for _, v := range variants {
			notApplied[v.ID] = "function has no body"
		}
		return nil
	}
	var guards []ast.Stmt
	var applied []int
	for _, v := range variants {
		mfn, ok := v.Mutated.(*ast.FuncDecl)
		if !ok || mfn.Body == nil || len(mfn.Body.List) != 0 {
			notApplied[v.ID] = "only empty-body function mutations are supported"
			continue
		}
		if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
			notApplied[v.ID] = "function has results; an empty body would not compile"
			continue
		}
		g := guardIf(v.ID, &ast.ReturnStmt{})
		FillPositions(g, fn.Body.Lbrace)
		guards = append(guards, g)
		applied = append(applied, v.ID)
	}
	if len(guards) > 0 {
		fn.Body.List = append(guards, fn.Body.List...)
	}
	return applied
}

func injectStmt(orig ast.Stmt, variants []Variant, notApplied map[int]string) (ast.Node, []int, map[int]string) {
	type pair struct {
		id   int
		stmt ast.Stmt
	}
	var pairs []pair
	for _, v := range variants {
		ms, ok := v.Mutated.(ast.Stmt)
		if !ok {
			notApplied[v.ID] = fmt.Sprintf("statement mutation produced %T", v.Mutated)
			continue
		}
		pairs = append(pairs, pair{v.ID, ms})
	}
	if len(pairs) == 0 {
		return orig, nil, notApplied
	}
	var chain ast.Stmt = &ast.BlockStmt{List: []ast.Stmt{orig}}
	for i := len(pairs) - 1; i >= 0; i-- {
		chain = &ast.IfStmt{
			Cond: idEquals(pairs[i].id),
			Body: &ast.BlockStmt{List: []ast.Stmt{pairs[i].stmt}},
			Else: chain,
		}
	}
	applied := make([]int, len(pairs))
	for i, p := range pairs {
		applied[i] = p.id
	}
	return chain, applied, notApplied
}

func injectExpr(orig ast.Expr, variants []Variant, exprType string, notApplied map[int]string) (ast.Node, []int, map[int]string) {
	if exprType == "" {
		for _, v := range variants {
			notApplied[v.ID] = "expression type unknown"
		}
		return orig, nil, notApplied
	}
	typeExpr, err := parseTypeExpr(exprType)
	if err != nil {
		for _, v := range variants {
			notApplied[v.ID] = fmt.Sprintf("cannot spell expression type %q", exprType)
		}
		return orig, nil, notApplied
	}
	stmts := make([]ast.Stmt, 0, len(variants)+1)
	var applied []int
	for _, v := range variants {
		me, ok := v.Mutated.(ast.Expr)
		if !ok {
			notApplied[v.ID] = fmt.Sprintf("expression mutation produced %T", v.Mutated)
			continue
		}
		stmts = append(stmts, guardIf(v.ID, &ast.ReturnStmt{Results: []ast.Expr{me}}))
		applied = append(applied, v.ID)
	}
	if len(applied) == 0 {
		return orig, nil, notApplied
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{orig}})
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{},
				Results: &ast.FieldList{List: []*ast.Field{{Type: typeExpr}}},
			},
			Body: &ast.BlockStmt{List: stmts},
		},
	}, applied, notApplied
}

func guardIf(id int, stmt ast.Stmt) *ast.IfStmt {
	return &ast.IfStmt{Cond: idEquals(id), Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}}
}

func paren(e ast.Expr) ast.Expr {
	switch e.(type) {
	case *ast.Ident, *ast.BasicLit, *ast.ParenExpr, *ast.CallExpr, *ast.SelectorExpr:
		return e
	}
	return &ast.ParenExpr{X: e}
}

func idEquals(id int) ast.Expr {
	return &ast.BinaryExpr{
		X:  &ast.Ident{Name: "activeMutantID"},
		Op: token.EQL,
		Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(id)},
	}
}

func idNotEquals(id int) ast.Expr {
	return &ast.BinaryExpr{
		X:  &ast.Ident{Name: "activeMutantID"},
		Op: token.NEQ,
		Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(id)},
	}
}

func inferExprType(expr ast.Expr, siteReturnType string, typeMap map[string]string) string {
	single := siteReturnType
	if strings.Contains(single, ",") {
		single = ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if typeMap != nil {
			if typ, ok := typeMap[e.Name]; ok && typ != "func:opaque" {
				return typ
			}
		}
		switch e.Name {
		case "true", "false":
			return "bool"
		case "nil":
			return ""
		}
		return single
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
		case token.IMAG:
			return "complex128"
		}
	case *ast.BinaryExpr:
		if isComparisonOp(e.Op) || isLogicalOp(e.Op) {
			return "bool"
		}
		if e.Op == token.SHL || e.Op == token.SHR {
			return inferExprType(e.X, siteReturnType, typeMap)
		}
		x := inferExprType(e.X, siteReturnType, typeMap)
		y := inferExprType(e.Y, siteReturnType, typeMap)
		_, xLit := e.X.(*ast.BasicLit)
		_, yLit := e.Y.(*ast.BasicLit)
		switch {
		case xLit && !yLit && y != "":
			return y
		case x != "":
			return x
		default:
			return y
		}
	case *ast.UnaryExpr:
		switch e.Op {
		case token.NOT:
			return "bool"
		case token.AND:
			if inner := inferExprType(e.X, siteReturnType, typeMap); inner != "" {
				return "*" + inner
			}
			return ""
		}
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.StarExpr:
		inner := inferExprType(e.X, siteReturnType, typeMap)
		return strings.TrimPrefix(inner, "*")
	case *ast.CompositeLit:
		return analysis.TypeExprToString(e.Type)
	case *ast.ParenExpr:
		return inferExprType(e.X, siteReturnType, typeMap)
	case *ast.CallExpr:
		return single
	}
	return single
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
