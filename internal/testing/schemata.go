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
		return nil, err
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
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		astFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		changed := false
		for _, imp := range astFile.Imports {
			if strings.HasPrefix(imp.Path.Value, "\"github.com/aclfe/gorgon/") {
				imp.Path.Value = strings.Replace(imp.Path.Value, "\"github.com/aclfe/gorgon/", "\"gorgon-bench/", 1)
				changed = true
			}
		}

		if changed {
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, astFile); err != nil {
				return fmt.Errorf("format %s: %w", path, err)
			}
			if err := os.WriteFile(path, buf.Bytes(), filePermissions); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk failed: %w", err)
	}
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
		content = strings.Replace(content, "module github.com/aclfe/gorgon", "module gorgon-bench", 1)
	}

	if err := os.WriteFile(goModPath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy failed:\n%s", out)
	}

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
		Line   int
		Column int
	}
	posToMutants := make(map[posKey][]schemata_nodes.MutantForSite)
	for _, mutant := range fileMutants {
		key := posKey{Line: mutant.Site.Line, Column: mutant.Site.Column}
		posToMutants[key] = append(posToMutants[key], schemata_nodes.MutantForSite{
			ID:         mutant.ID,
			Op:         mutant.Operator,
			ReturnType: mutant.Site.ReturnType,
		})
	}

	astutil.Apply(file, nil, func(cursor *astutil.Cursor) bool {
		node := cursor.Node()
		if node == nil {
			return true
		}
		var nodePos token.Position
		if be, ok := node.(*ast.BinaryExpr); ok {
			nodePos = fset.Position(be.OpPos)
		} else if cc, ok := node.(*ast.CaseClause); ok {
			nodePos = fset.Position(cc.Case)
		} else {
			nodePos = fset.Position(node.Pos())
		}
		if !nodePos.IsValid() {
			return true
		}

		key := posKey{Line: nodePos.Line, Column: nodePos.Column}
		if mutants, ok := posToMutants[key]; ok {
			returnType := ""
			if len(mutants) > 0 {
				returnType = mutants[0].ReturnType
			}
			schemata := createSchemataExpr(node, mutants, returnType, file)
			cursor.Replace(schemata)
		}
		return true
	})

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return fmt.Errorf("format failed: %w", err)
	}
	if err := os.WriteFile(filePath, buf.Bytes(), filePermissions); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

func createSchemataExpr(original ast.Node, mutants []schemata_nodes.MutantForSite, returnType string, file *ast.File) ast.Node {
	handler := schemata_nodes.GetHandler(original)
	if handler != nil {
		return handler(original, mutants, returnType, file)
	}

	return handleExpression(original, mutants, returnType, file)
}

func handleExpression(original ast.Node, mutants []schemata_nodes.MutantForSite, returnType string, file *ast.File) ast.Node {
	resultType := inferResultType(original)

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

func InjectSchemataHelpers(_ string, fileToMutants map[string][]*Mutant) error {
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
		if id, err := strconv.Atoi(idStr); err == nil {
			activeMutantID = id
		}
	}
}
`, pkgName)

		if err := os.WriteFile(filepath.Join(pkgDir, "gorgon_schemata.go"), []byte(helper), filePermissions); err != nil {
			return fmt.Errorf("failed to write helper: %w", err)
		}
	}
	return nil
}
