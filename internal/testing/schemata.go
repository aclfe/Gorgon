// Package testing provides testing utilities and schema-based mutation logic.
package testing

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/ast/astutil"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/mutator"
)

const (
	filePermissions = 0o600
)

// GenerateAndRunSchemata is the new blazing-fast mutation testing path using schemata.
//
//nolint:gocognit,gocyclo,cyclop,funlen
func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int) ([]Mutant, error) {
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].File.Name() < sites[j].File.Name()
	})

	var mutants []Mutant
	mutantID := 1
	for _, site := range sites {
		for _, op := range operators {
			apply := false
			if cop, ok := op.(mutator.ContextualOperator); ok {
				ctx := mutator.Context{ReturnType: site.ReturnType}
				apply = cop.CanApplyWithContext(site.Node, ctx)
			} else {
				apply = op.CanApply(site.Node)
			}
			if apply {
				mutants = append(mutants, Mutant{
					ID:       mutantID,
					Site:     site,
					Operator: op,
				})
				mutantID++
			}
		}
	}
	if len(mutants) == 0 {
		return nil, nil
	}

	modPath := findGoMod(baseDir)
	moduleRoot := baseDir
	if modPath != "" {
		moduleRoot = filepath.Dir(modPath)
	}

	absModule, err := filepath.Abs(moduleRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for module root: %w", err)
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for base dir: %w", err)
	}

	_, err = filepath.Rel(absModule, absBase)
	if err != nil {
		return nil, fmt.Errorf("failed to compute relative path: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "gorgon-schemata-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if err := CopyDir(moduleRoot, tempDir); err != nil {
		return nil, fmt.Errorf("failed to copy module: %w", err)
	}

	if err := RewriteImports(tempDir); err != nil {
		return nil, fmt.Errorf("rewrite imports: %w", err)
	}

	if err := MakeSelfContained(tempDir); err != nil {
		// Don't fail on mod tidy issues, just continue
		fmt.Fprintf(os.Stderr, "Warning: MakeSelfContained had issues: %v\n", err)
	}

	fileToMutants := make(map[string][]*Mutant)
	for i := range mutants {
		mutant := &mutants[i]
		rel, err := filepath.Rel(absModule, mutant.Site.File.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path for %s: %w", mutant.Site.File.Name(), err)
		}
		tempFile := filepath.Join(tempDir, rel)
		fileToMutants[tempFile] = append(fileToMutants[tempFile], mutant)
	}

	for tempFile, fileMutants := range fileToMutants {
		if err := ApplySchemataToFile(tempFile, fileMutants); err != nil {
			return nil, fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}

	if err := InjectSchemataHelpers(tempDir, fileToMutants); err != nil {
		return nil, err
	}

	pkgToBinary := make(map[string]string)
	for tempFile := range fileToMutants {
		pkgDir := filepath.Dir(tempFile)
		if _, exists := pkgToBinary[pkgDir]; exists {
			continue
		}

		relPkg, err := filepath.Rel(tempDir, pkgDir)
		if err != nil {
			return nil, fmt.Errorf("failed to compute relative path: %w", err)
		}
		if relPkg == "." {
			relPkg = ""
		} else {
			relPkg = "./" + filepath.ToSlash(relPkg)
		}

		testBinary := filepath.Join(pkgDir, "package.test")

		if strings.Contains(relPkg, "\n") || strings.Contains(relPkg, "\r") {
			return nil, fmt.Errorf("invalid package path contains newline: %s", relPkg)
		}

		cmd := exec.Command("go", "test", "-c", "-o", testBinary, relPkg)
		cmd.Dir = tempDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("test compilation failed for %s:\n%s", relPkg, out)
		}

		pkgToBinary[pkgDir] = testBinary
	}

	if concurrent == 0 {
		concurrent = runtime.NumCPU()
	}

	type mutantResult struct {
		id     int
		status string
		err    error
	}

	resultsChan := make(chan mutantResult, len(mutants))

	pkgToMutantIDs := make(map[string][]int)
	mutantIDToIndex := make(map[int]int)
	for idx := range mutants {
		mutant := &mutants[idx]
		relFile, err := filepath.Rel(absModule, mutant.Site.File.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path: %w", err)
		}
		pkgDir := filepath.Join(tempDir, filepath.Dir(relFile))
		pkgToMutantIDs[pkgDir] = append(pkgToMutantIDs[pkgDir], mutant.ID)
		mutantIDToIndex[mutant.ID] = idx
	}

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.SetLimit(concurrent)
	for pkgDir, mutantIDs := range pkgToMutantIDs {
		errGroup.Go(func(pkgDir string, mutantIDs []int) func() error {
			return func() error {
				testBinary := pkgToBinary[pkgDir]

				for _, mutantID := range mutantIDs {
					cmd := exec.CommandContext(ctx, testBinary, "-test.timeout=10s")
					cmd.Dir = pkgDir
					cmd.Env = append(os.Environ(), fmt.Sprintf("GORGON_MUTANT_ID=%d", mutantID))

					out, err := cmd.CombinedOutput()
					status := "survived"
					var errMsg error
					if err != nil {
						status = "killed"
						errMsg = fmt.Errorf("%s", out)
					}

					resultsChan <- mutantResult{id: mutantID, status: status, err: errMsg}
				}
				return nil
			}
		}(pkgDir, mutantIDs))
	}

	go func() {
		_ = errGroup.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		idx := mutantIDToIndex[result.id]
		mutants[idx].Status = result.status
		mutants[idx].Error = result.err
		mutants[idx].TempDir = tempDir
	}

	if err := errGroup.Wait(); err != nil {
		return nil, fmt.Errorf("wait failed: %w", err)
	}

	return mutants, nil
}

func RewriteImports(tempDir string) error {
	// No longer needed - we use replace directive instead of renaming module
	// This preserves all transitive dependency paths
	return nil
}

func MakeSelfContained(tempDir string) error {
	goModPath := filepath.Join(tempDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read go.mod: %w", err)
	}

	content := string(data)
	if os.IsNotExist(err) {
		content = "module gorgon-bench\ngo 1.21\n"
	} else {
		// Keep original module name but add replace directive
		// This avoids breaking transitive dependencies
		if !strings.Contains(content, "replace github.com/aclfe/gorgon =>") {
			content = strings.TrimSpace(content) + "\n\nreplace github.com/aclfe/gorgon => ./\n"
		}
	}

	if err := os.WriteFile(goModPath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	// Don't remove go.sum - use existing resolved dependencies
	// This avoids re-resolving deps that may have path conflicts

	// Skip go mod tidy entirely - deps are already resolved
	// The benchmark copy is temporary and isolated

	return nil
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

	type posKey struct {
		Line     int
		Column   int
		NodeType string
	}
	posToMutants := make(map[posKey][]schemata_nodes.MutantForSite)
	for _, mutant := range fileMutants {
		nodeType := fmt.Sprintf("%T", mutant.Site.Node)
		key := posKey{Line: mutant.Site.Line, Column: mutant.Site.Column, NodeType: nodeType}
		posToMutants[key] = append(posToMutants[key], schemata_nodes.MutantForSite{
			ID:         mutant.ID,
			Op:         mutant.Operator,
			ReturnType: mutant.Site.ReturnType,
			NodeType:   nodeType,
		})
	}

	recoveredErr := make(chan error, 1)
	func() {
		defer func() {
			if r := recover(); r != nil {
				recoveredErr <- fmt.Errorf("panic during schemata application: %v", r)
			}
		}()

		changesMade := 0
		astutil.Apply(file, nil, func(cursor *astutil.Cursor) bool {
			node := cursor.Node()
			if node == nil {
				return true
			}

			// Skip nodes inside import declarations
			if cursor.Parent() != nil {
				if _, ok := cursor.Parent().(*ast.ImportSpec); ok {
					return true
				}
			}

			var nodePos token.Position
			if be, ok := node.(*ast.BinaryExpr); ok {
				nodePos = fset.Position(be.OpPos)
			} else if cc, ok := node.(*ast.CaseClause); ok {
				nodePos = fset.Position(cc.Case)
			} else if ids, ok := node.(*ast.IncDecStmt); ok {
				nodePos = fset.Position(ids.TokPos)
			} else {
				nodePos = fset.Position(node.Pos())
			}
			if !nodePos.IsValid() {
				return true
			}

			nodeType := fmt.Sprintf("%T", node)
			key := posKey{Line: nodePos.Line, Column: nodePos.Column, NodeType: nodeType}
			if mutants, ok := posToMutants[key]; ok {
				returnType := ""
				if len(mutants) > 0 {
					returnType = mutants[0].ReturnType
				}
				schemata := createSchemataExpr(node, mutants, returnType, file)
				if schemata != nil && schemata != node {
					if isValidReplacement(node, schemata) {
						cursor.Replace(schemata)
						changesMade++
					}
				}
			}
			return true
		})
		_ = changesMade
		recoveredErr <- nil
	}()

	if err := <-recoveredErr; err != nil {
		return err
	}

	originalImports := make(map[string]*ast.ImportSpec)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		originalImports[path] = imp
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		// Format failed - write original source back to avoid compilation errors
		_ = os.WriteFile(filePath, src, filePermissions)
		return nil  // Don't fail, just skip this file
	}

	fset2 := token.NewFileSet()
	file2, err := parser.ParseFile(fset2, filePath, buf.Bytes(), parser.ParseComments)
	if err == nil {
		// Check which identifiers are used in the mutated code
		usedIdents := make(map[string]bool)
		ast.Inspect(file2, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				usedIdents[ident.Name] = true
			}
			return true
		})

		// For each original import, check if the package is still used
		for path, origImp := range originalImports {
			parts := strings.Split(path, "/")
			pkgName := parts[len(parts)-1]
			if origImp.Name != nil && origImp.Name.Name != "_" {
				pkgName = origImp.Name.Name
			}

			// If package not used, change import to blank
			if !usedIdents[pkgName] {
				for _, existing := range file2.Imports {
					if existing.Path.Value == "\""+path+"\"" {
						existing.Name = &ast.Ident{Name: "_"}
						break
					}
				}
			}
		}

		buf.Reset()
		if err := format.Node(&buf, fset2, file2); err != nil {
			return fmt.Errorf("format after import fix failed: %w", err)
		}
	}

	if err := os.WriteFile(filePath, buf.Bytes(), filePermissions); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

func createSchemataExpr(original ast.Node, mutants []schemata_nodes.MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	handler := schemata_nodes.GetHandler(original)
	if handler != nil {
		return handler(original, mutants, returnType, file)
	}

	// Don't use generic expression wrapping - rely on specific handlers
	return original
}

func isValidReplacement(original, replacement ast.Node) bool {
	if original == nil || replacement == nil {
		return false
	}

	typeOriginal := fmt.Sprintf("%T", original)
	typeReplacement := fmt.Sprintf("%T", replacement)

	if typeOriginal == typeReplacement {
		return true
	}

	validReplacements := map[string][]string{
		"*ast.BinaryExpr":   {"*ast.BinaryExpr", "*ast.CallExpr"},
		"*ast.UnaryExpr":    {"*ast.UnaryExpr", "*ast.CallExpr"},
		"*ast.CallExpr":     {"*ast.CallExpr"},
		"*ast.Ident":        {"*ast.Ident", "*ast.CallExpr", "*ast.BasicLit"},
		"*ast.BasicLit":     {"*ast.BasicLit", "*ast.Ident", "*ast.CallExpr"},
		"*ast.IfStmt":       {"*ast.IfStmt", "*ast.BlockStmt"},
		"*ast.ForStmt":      {"*ast.ForStmt", "*ast.BlockStmt"},
		"*ast.RangeStmt":    {"*ast.RangeStmt", "*ast.BlockStmt"},
		"*ast.AssignStmt":   {"*ast.AssignStmt", "*ast.BlockStmt", "*ast.ExprStmt"},
		"*ast.ReturnStmt":   {"*ast.ReturnStmt", "*ast.BlockStmt", "*ast.EmptyStmt"},
		"*ast.DeferStmt":    {"*ast.DeferStmt", "*ast.EmptyStmt", "*ast.ExprStmt", "*ast.BlockStmt"},
		"*ast.BranchStmt":   {"*ast.BranchStmt", "*ast.EmptyStmt", "*ast.ExprStmt", "*ast.BlockStmt"},
		"*ast.GoStmt":       {"*ast.GoStmt", "*ast.EmptyStmt", "*ast.ExprStmt", "*ast.BlockStmt"},
		"*ast.ExprStmt":     {"*ast.ExprStmt", "*ast.BlockStmt", "*ast.EmptyStmt"},
		"*ast.IncDecStmt":   {"*ast.IncDecStmt", "*ast.CallExpr"},
		"*ast.SendStmt":     {"*ast.SendStmt", "*ast.CallExpr"},
		"*ast.SwitchStmt":   {"*ast.SwitchStmt", "*ast.BlockStmt"},
		"*ast.TypeSwitchStmt": {"*ast.TypeSwitchStmt", "*ast.BlockStmt"},
		"*ast.SelectStmt":   {"*ast.SelectStmt", "*ast.BlockStmt"},
		"*ast.CommClause":   {"*ast.CommClause", "*ast.BlockStmt"},
		"*ast.LabeledStmt":  {"*ast.LabeledStmt", "*ast.BlockStmt"},
		"*ast.DeclStmt":     {"*ast.DeclStmt", "*ast.BlockStmt"},
		"*ast.EmptyStmt":    {"*ast.EmptyStmt", "*ast.BlockStmt"},
		"*ast.BlockStmt":    {"*ast.BlockStmt"},
		"*ast.CaseClause":   {"*ast.CaseClause"},
		"*ast.FuncDecl":     {"*ast.FuncDecl"},
	}

	if validTypes, ok := validReplacements[typeOriginal]; ok {
		for _, t := range validTypes {
			if typeReplacement == t {
				return true
			}
		}
	}

	return false
}

func handleExpression(original ast.Node, mutants []schemata_nodes.MutantForSite, returnType string, file *ast.File) ast.Node {
	if _, isExpr := original.(ast.Expr); !isExpr {
		return original
	}

	resultType := returnType
	if resultType == "" {
		resultType = "interface{}"
	}

	stmts := make([]ast.Stmt, 0, len(mutants)+1)

	for _, mutant := range mutants {
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated == nil {
			continue
		}

		mutatedExpr, ok := mutated.(ast.Expr)
		if !ok {
			continue
		}

		retStmt := &ast.ReturnStmt{Results: []ast.Expr{mutatedExpr}}

		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutant.ID)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{retStmt},
			},
		})
	}

	var originalExpr ast.Expr
	switch n := original.(type) {
	case ast.Expr:
		originalExpr = n
	default:
		originalExpr = &ast.Ident{Name: "nil"}
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{originalExpr}})

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Results: &ast.FieldList{
					List: []*ast.Field{
						{Type: &ast.Ident{Name: resultType}},
					},
				},
			},
			Body: &ast.BlockStmt{List: stmts},
		},
	}
}

func inferResultType(node ast.Node) string {
	switch node.(type) {
	case *ast.BinaryExpr:
		be := node.(*ast.BinaryExpr)
		if isComparisonOp(be.Op) {
			return "bool"
		}
		return "int"
	case *ast.IncDecStmt:
		return "int"
	case *ast.ReturnStmt:
		return "interface{}"
	default:
		return "interface{}"
	}
}

func isComparisonOp(op token.Token) bool {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	}
	return false
}

func InjectSchemataHelpers(pkgDir string, fileToMutants map[string][]*Mutant) error {
	pkgToFiles := make(map[string][]string)
	for tempFile := range fileToMutants {
		pkgDir := filepath.Dir(tempFile)
		pkgToFiles[pkgDir] = append(pkgToFiles[pkgDir], tempFile)
	}

	for pkgDir, files := range pkgToFiles {
		if len(files) == 0 {
			continue
		}

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
