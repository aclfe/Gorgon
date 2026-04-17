package testing

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/aclfe/gorgon/internal/core/schemata_nodes"
)

var formatBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

//

var validNodeTypeReplacements = map[schemata_nodes.NodeType][]schemata_nodes.NodeType{
	schemata_nodes.NTBinaryExpr:     {schemata_nodes.NTBinaryExpr, schemata_nodes.NTCallExpr},
	schemata_nodes.NTUnaryExpr:      {schemata_nodes.NTUnaryExpr, schemata_nodes.NTCallExpr},
	schemata_nodes.NTCallExpr:       {schemata_nodes.NTCallExpr},
	schemata_nodes.NTIdent:          {schemata_nodes.NTIdent, schemata_nodes.NTCallExpr, schemata_nodes.NTBasicLit},
	schemata_nodes.NTBasicLit:       {schemata_nodes.NTBasicLit, schemata_nodes.NTCallExpr, schemata_nodes.NTIdent},
	schemata_nodes.NTCaseClause:     {schemata_nodes.NTCaseClause},
	schemata_nodes.NTIfStmt:         {schemata_nodes.NTIfStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTForStmt:        {schemata_nodes.NTForStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTRangeStmt:      {schemata_nodes.NTRangeStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTAssignStmt:     {schemata_nodes.NTAssignStmt, schemata_nodes.NTBlockStmt, schemata_nodes.NTExprStmt},
	schemata_nodes.NTIncDecStmt:     {schemata_nodes.NTIncDecStmt, schemata_nodes.NTCallExpr, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTDeferStmt:      {schemata_nodes.NTDeferStmt, schemata_nodes.NTExprStmt, schemata_nodes.NTErrStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTGoStmt:         {schemata_nodes.NTGoStmt, schemata_nodes.NTExprStmt, schemata_nodes.NTErrStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTSendStmt:       {schemata_nodes.NTSendStmt, schemata_nodes.NTCallExpr, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTSwitchStmt:     {schemata_nodes.NTSwitchStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTTypeSwitchStmt: {schemata_nodes.NTTypeSwitchStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTReturnStmt:     {schemata_nodes.NTReturnStmt, schemata_nodes.NTBlockStmt, schemata_nodes.NTErrStmt},
	schemata_nodes.NTBranchStmt:     {schemata_nodes.NTBranchStmt, schemata_nodes.NTExprStmt, schemata_nodes.NTErrStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTSelectStmt:     {schemata_nodes.NTSelectStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTCommClause:     {schemata_nodes.NTCommClause, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTLabeledStmt:    {schemata_nodes.NTLabeledStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTExprStmt:       {schemata_nodes.NTExprStmt, schemata_nodes.NTBlockStmt, schemata_nodes.NTErrStmt},
	schemata_nodes.NTDeclStmt:       {schemata_nodes.NTDeclStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTErrStmt:        {schemata_nodes.NTErrStmt, schemata_nodes.NTBlockStmt},
	schemata_nodes.NTBlockStmt:      {schemata_nodes.NTBlockStmt},
	schemata_nodes.NTFuncDecl:       {schemata_nodes.NTFuncDecl},
}

func ApplySchemataToFile(filePath string, fileMutants []*Mutant) (map[int]PositionMapping, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	return applySchemataToAST(file, fset, filePath, src, fileMutants)
}

type PositionMapping struct {
	OriginalLine int
	OriginalCol  int
	TempLine     int
	TempCol      int
}

func ApplySchemataToAST(fileAST *ast.File, fset *token.FileSet, filePath string, src []byte, fileMutants []*Mutant) (map[int]PositionMapping, error) {
	return applySchemataToAST(fileAST, fset, filePath, src, fileMutants)
}

func buildPositionMapping(mutants []*Mutant, fset *token.FileSet) map[int]PositionMapping {
	result := make(map[int]PositionMapping, len(mutants))
	for _, m := range mutants {
		if m.Site.Node == nil {
			continue
		}
		origPos := fset.Position(m.Site.Node.Pos())
		tempNode := findNodeByID(mutants, m.ID)
		if tempNode != nil {
			tempPos := fset.Position(tempNode.Pos())
			result[m.ID] = PositionMapping{
				OriginalLine: origPos.Line,
				OriginalCol:  origPos.Column,
				TempLine:     tempPos.Line,
				TempCol:      tempPos.Column,
			}
		} else {
			result[m.ID] = PositionMapping{
				OriginalLine: origPos.Line,
				OriginalCol:  origPos.Column,
				TempLine:     origPos.Line,
				TempCol:      origPos.Column,
			}
		}
	}
	return result
}

func findNodeByID(mutants []*Mutant, id int) ast.Node {
	for _, m := range mutants {
		if m.ID == id {
			return m.Site.Node
		}
	}
	return nil
}

func extractMutantIDs(mutants []*Mutant) []int {
	ids := make([]int, len(mutants))
	for i, m := range mutants {
		ids[i] = m.ID
	}
	return ids
}

//
//	if activeMutantID == 42 {
//

func findMutantPositionsInFile(filePath string, mutantIDs []int, originalPositions map[int]PositionMapping) map[int]PositionMapping {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return originalPositions
	}
	lines := bytes.Split(content, []byte{'\n'})

	result := make(map[int]PositionMapping, len(mutantIDs))

	for id, pos := range originalPositions {
		result[id] = pos
	}

	for _, id := range mutantIDs {
		pattern := []byte(fmt.Sprintf("activeMutantID == %d", id))
		for lineNum, line := range lines {
			if bytes.Contains(line, pattern) {

				result[id] = PositionMapping{
					OriginalLine: originalPositions[id].OriginalLine,
					OriginalCol:  originalPositions[id].OriginalCol,
					TempLine:     lineNum + 1,
					TempCol:      bytes.Index(line, pattern) + 1,
				}
				break
			}
		}
	}

	return result
}

func applySchemataToAST(file *ast.File, fset *token.FileSet, filePath string, src []byte, fileMutants []*Mutant) (map[int]PositionMapping, error) {

	posToMutants := buildPositionToMutantsMap(fileMutants)

	constNodes := findConstNodes(file)

	astutil.Apply(file, nil, func(cursor *astutil.Cursor) bool {
		return applySchemataVisitor(cursor, fset, posToMutants, constNodes, file, nil)
	})

	fixUnusedImports(file)
	fixUnusedLoopVarsAfterMutationFast(file)

	buf := formatBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := format.Node(buf, fset, file); err != nil {
		formatBufPool.Put(buf)

		if len(src) > 0 {
			_ = os.WriteFile(filePath, src, filePermissions)
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] format.Node failed for %s and original source is empty, skipping restore\n", filePath)
		}
		return nil, nil
	}

	if buf.Len() == 0 {
		formatBufPool.Put(buf)
		if len(src) > 0 {
			_ = os.WriteFile(filePath, src, filePermissions)
			fmt.Fprintf(os.Stderr, "[WARN] formatted output for %s is empty, restoring original source\n", filePath)
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] formatted output for %s is empty and no original source available\n", filePath)
		}
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		formatBufPool.Put(buf)
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), filePermissions); err != nil {
		formatBufPool.Put(buf)
		return nil, fmt.Errorf("write failed: %w", err)
	}
	formatBufPool.Put(buf)

	origPositions := buildPositionMapping(fileMutants, fset)
	mutantIDs := extractMutantIDs(fileMutants)
	positionMap := findMutantPositionsInFile(filePath, mutantIDs, origPositions)
	return positionMap, nil
}

func InjectSchemataHelpers(fileToMutants map[string][]*Mutant) error {

	type pkgInfo struct {
		files   []string
		pkgName string
	}
	pkgToInfo := make(map[string]*pkgInfo, len(fileToMutants))
	for tempFile, mutants := range fileToMutants {
		dir := filepath.Dir(tempFile)
		info, ok := pkgToInfo[dir]
		if !ok {
			info = &pkgInfo{}
			pkgToInfo[dir] = info
		}
		info.files = append(info.files, tempFile)

		if info.pkgName == "" {
			for _, m := range mutants {
				if m.Site.FileAST != nil && m.Site.FileAST.Name != nil {
					info.pkgName = m.Site.FileAST.Name.Name
					break
				}
			}
		}
	}

	dirs := make([]string, 0, len(pkgToInfo))
	for dir := range pkgToInfo {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	for _, dir := range dirs {
		info := pkgToInfo[dir]
		if len(info.files) == 0 {
			continue
		}

		pkgName := info.pkgName
		if pkgName == "" {
			pkgName = filepath.Base(dir)
		}

		const helperSentinel = "// GORGON_SCHEMATA"

		helper := fmt.Sprintf(`package %s


import (
	"os"
	"strconv"
)

var activeMutantID int

func init() {
	if idStr := os.Getenv("GORGON_MUTANT_ID"); idStr != "" {
		activeMutantID, _ = strconv.Atoi(idStr)
	}
}
`, pkgName)

		helperFile := filepath.Join(dir, "gorgon_schemata.go")
		existing, _ := os.ReadFile(helperFile)
		if bytes.Contains(existing, []byte(helperSentinel)) {
			if err := os.WriteFile(helperFile, []byte(helper), filePermissions); err != nil {
				return fmt.Errorf("failed to overwrite helper: %w", err)
			}
			continue
		}
		if err := os.WriteFile(helperFile, []byte(helper), filePermissions); err != nil {
			return fmt.Errorf("failed to write helper: %w", err)
		}
	}
	return nil
}

type posKey struct {
	Line   int
	Column int
	Type   uint8
}

func buildPositionToMutantsMap(mutants []*Mutant) map[posKey][]schemata_nodes.MutantForSite {
	posToMutants := make(map[posKey][]schemata_nodes.MutantForSite, len(mutants))
	for _, mutant := range mutants {
		key := posKey{
			Line:   mutant.Site.Line,
			Column: mutant.Site.Column,
			Type:   schemata_nodes.NodeTypeToUint8(mutant.Site.Node),
		}
		posToMutants[key] = append(posToMutants[key], schemata_nodes.MutantForSite{
			ID:            mutant.ID,
			Op:            mutant.Operator,
			ReturnType:    mutant.Site.ReturnType,
			EnclosingFunc: mutant.Site.EnclosingFunc,
		})
	}
	return posToMutants
}

func findConstNodes(file *ast.File) map[ast.Node]bool {
	constNodes := make(map[ast.Node]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			return true
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, val := range vs.Values {
				ast.Inspect(val, func(child ast.Node) bool {
					if child != nil {
						constNodes[child] = true
					}
					return true
				})
			}
		}
		return true
	})
	return constNodes
}

func applySchemataVisitor(cursor *astutil.Cursor, fset *token.FileSet, posToMutants map[posKey][]schemata_nodes.MutantForSite, constNodes map[ast.Node]bool, file *ast.File, visitedNodes map[*ast.Decl]bool) bool {
	node := cursor.Node()
	if node == nil {
		return true
	}

	if cursor.Parent() != nil {
		if _, ok := cursor.Parent().(*ast.ImportSpec); ok {
			return true
		}
	}

	if constNodes[node] {
		return true
	}

	newPos := schemata_nodes.GetNodePosition(node, fset)
	key := posKey{Line: newPos.Line, Column: newPos.Column, Type: schemata_nodes.NodeTypeToUint8(node)}
	mutants, ok := posToMutants[key]
	if !ok {
		return true
	}

	returnType := ""
	if len(mutants) > 0 {
		returnType = mutants[0].ReturnType
	}

	schemata := createSchemataExpr(node, mutants, returnType, file)
	if schemata == nil || schemata == node {
		return true
	}

	if _, isExpr := node.(ast.Expr); isExpr {
		if _, ok := schemata.(ast.Expr); ok {
			safeReplace(cursor, schemata)
		}
	} else if isValidReplacement(node, schemata) {
		safeReplace(cursor, schemata)
	}

	return true
}

func safeReplace(cursor *astutil.Cursor, replacement ast.Node) {
	defer func() { _ = recover() }()
	cursor.Replace(replacement)
}

func createSchemataExpr(original ast.Node, mutants []schemata_nodes.MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	handler := schemata_nodes.GetHandler(original)
	if handler != nil {
		return handler(original, mutants, returnType, file)
	}

	return original
}

func isValidReplacement(original, replacement ast.Node) bool {
	if original == nil || replacement == nil {
		return false
	}

	typeOriginal := schemata_nodes.NodeTypeOf(original)
	typeReplacement := schemata_nodes.NodeTypeOf(replacement)

	if typeOriginal == typeReplacement {
		return true
	}

	if validTypes, ok := validNodeTypeReplacements[typeOriginal]; ok {
		for _, t := range validTypes {
			if typeReplacement == t {
				return true
			}
		}
	}

	return false
}

func fixUnusedImports(file *ast.File) {
	used := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		if _, ok := n.(*ast.ImportSpec); ok {
			return false
		}
		if ident, ok := n.(*ast.Ident); ok {
			used[ident.Name] = true
		}
		return true
	})

	for _, imp := range file.Imports {
		if imp.Name != nil && imp.Name.Name == "_" {
			continue
		}
		pkgName := getPackageNameFromImport(imp)
		if !used[pkgName] {

			if imp.Name == nil {
				imp.Name = &ast.Ident{Name: "_", NamePos: imp.Path.Pos()}
			}
		}
	}
}

func fixUnusedLoopVarsAfterMutationFast(file *ast.File) {

	for _, decl := range file.Decls {
		processDeclForLoopVars(decl)
	}
}

func processDeclForLoopVars(decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Body == nil {
			return
		}
		processStmtsForLoopVars(d.Body.List)
	case *ast.GenDecl:

	}
}

func processStmtsForLoopVars(stmts []ast.Stmt) {
	for i, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.RangeStmt:
			fixRangeStmt(s)
		case *ast.ForStmt:
			fixForStmt(s)
		case *ast.IfStmt:
			if s.Body != nil {
				processStmtsForLoopVars(s.Body.List)
			}
			if s.Else != nil {
				if block, ok := s.Else.(*ast.BlockStmt); ok {
					processStmtsForLoopVars(block.List)
				}
			}
		case *ast.BlockStmt:
			processStmtsForLoopVars(s.List)
		case *ast.CaseClause:
			processStmtsForLoopVars(s.Body)
		case *ast.CommClause:
			processStmtsForLoopVars(s.Body)
		}
		_ = i
	}
}

func fixRangeStmt(rangeStmt *ast.RangeStmt) {
	if rangeStmt.Body == nil {
		return
	}

	processStmtsForLoopVars(rangeStmt.Body.List)

	var loopVars []*ast.Ident
	if key, ok := rangeStmt.Key.(*ast.Ident); ok && key.Name != "_" {
		loopVars = append(loopVars, key)
	}
	if value, ok := rangeStmt.Value.(*ast.Ident); ok && value.Name != "_" {
		loopVars = append(loopVars, value)
	}

	for _, loopVar := range loopVars {
		if !isVariableUsedInBlockFast(loopVar.Name, rangeStmt.Body, loopVar) {
			blankAssign := &ast.AssignStmt{
				Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.Ident{Name: loopVar.Name}},
			}
			rangeStmt.Body.List = append([]ast.Stmt{blankAssign}, rangeStmt.Body.List...)
		}
	}
}

func fixForStmt(forStmt *ast.ForStmt) {
	if forStmt.Body == nil {
		return
	}

	processStmtsForLoopVars(forStmt.Body.List)

	var loopVars []*ast.Ident
	if init, ok := forStmt.Init.(*ast.AssignStmt); ok {
		for _, lhs := range init.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
				loopVars = append(loopVars, ident)
			}
		}
	}

	for _, loopVar := range loopVars {
		if !isVariableUsedInBlockFast(loopVar.Name, forStmt.Body, loopVar) {
			blankAssign := &ast.AssignStmt{
				Lhs: []ast.Expr{&ast.Ident{Name: "_"}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.Ident{Name: loopVar.Name}},
			}
			forStmt.Body.List = append([]ast.Stmt{blankAssign}, forStmt.Body.List...)
		}
	}
}

func isVariableUsedInBlockFast(name string, block *ast.BlockStmt, declIdent *ast.Ident) bool {
	if block == nil {
		return false
	}

	used := false
	ast.Inspect(block, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {

			if declIdent != nil && ident.Pos() == declIdent.Pos() {
				return true
			}
			used = true
			return false
		}
		return true
	})
	return used
}

func getPackageNameFromImport(imp *ast.ImportSpec) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	path := strings.Trim(imp.Path.Value, `"`)
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
