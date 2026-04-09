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

	"golang.org/x/tools/go/ast/astutil"

	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
)




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



func ApplySchemataToFile(filePath string, fileMutants []*Mutant) error {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filePath, err)
	}

	return applySchemataToAST(file, fset, filePath, src, fileMutants)
}




func ApplySchemataToAST(fileAST *ast.File, fset *token.FileSet, filePath string, src []byte, fileMutants []*Mutant) error {
	return applySchemataToAST(fileAST, fset, filePath, src, fileMutants)
}

func applySchemataToAST(file *ast.File, fset *token.FileSet, filePath string, src []byte, fileMutants []*Mutant) error {
	
	posToMutants := buildPositionToMutantsMap(fileMutants)

	
	constNodes := findConstNodes(file)

	
	astutil.Apply(file, nil, func(cursor *astutil.Cursor) bool {
		return applySchemataVisitor(cursor, fset, posToMutants, constNodes, file)
	})

	
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		_ = os.WriteFile(filePath, src, filePermissions)
		return nil
	}

	if err := os.WriteFile(filePath, buf.Bytes(), filePermissions); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}



func InjectSchemataHelpers(pkgDir string, fileToMutants map[string][]*Mutant) error {
	// Collect and sort keys deterministically
	pkgToFiles := make(map[string][]string, len(fileToMutants))
	for tempFile := range fileToMutants {
		pkgDir := filepath.Dir(tempFile)
		pkgToFiles[pkgDir] = append(pkgToFiles[pkgDir], tempFile)
	}

	// Sort package directories for deterministic processing
	pkgDirs := make([]string, 0, len(pkgToFiles))
	for pkgDir := range pkgToFiles {
		pkgDirs = append(pkgDirs, pkgDir)
	}
	sort.Strings(pkgDirs)

	for _, pkgDir := range pkgDirs {
		files := pkgToFiles[pkgDir]
		if len(files) == 0 {
			continue
		}

		// Sort files for deterministic package name detection
		sort.Strings(files)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, files[0], nil, parser.PackageClauseOnly)
		var pkgName string
		if err == nil && file.Name != nil {
			pkgName = file.Name.Name
		} else {
			pkgName = filepath.Base(pkgDir)
		}

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

		if err := os.WriteFile(filepath.Join(pkgDir, "gorgon_schemata.go"), []byte(helper), filePermissions); err != nil {
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

func applySchemataVisitor(cursor *astutil.Cursor, fset *token.FileSet, posToMutants map[posKey][]schemata_nodes.MutantForSite, constNodes map[ast.Node]bool, file *ast.File) bool {
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
