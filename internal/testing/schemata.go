package testing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/ast/astutil"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/mutator"
)

const (
	filePermissions = 0o600
)

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func GenerateMutants(sites []engine.Site, operators []mutator.Operator) []Mutant {
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].File.Name() < sites[j].File.Name()
	})

	type siteKey struct {
		file string
		line int
		col  int
		ntyp uint8
	}
	seen := make(map[siteKey]bool)
	uniqueSites := make([]engine.Site, 0, len(sites))
	for _, site := range sites {
		key := siteKey{
			file: site.File.Name(),
			line: site.Line,
			col:  site.Column,
			ntyp: TypeToUint8(site.Node),
		}
		if !seen[key] {
			seen[key] = true
			uniqueSites = append(uniqueSites, site)
		}
	}
	sites = uniqueSites

	var mutants []Mutant
	mutantID := 1
	for _, site := range sites {
		for _, op := range operators {
			apply := false
			if cop, ok := op.(mutator.ContextualOperator); ok {
				ctx := mutator.Context{ReturnType: site.ReturnType, EnclosingFunc: site.EnclosingFunc}
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
	return mutants
}

func testArgs(timeout string, tests []string) []string {
	args := []string{"-test.timeout=" + timeout}
	if len(tests) > 0 {
		pattern := strings.Join(tests, "|")
		args = append(args, "-test.run="+pattern)
	}
	return args
}

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int, cache *cache.Cache, tests []string) ([]Mutant, error) {
	mutants := GenerateMutants(sites, operators)
	if len(mutants) == 0 {
		return nil, nil
	}

	if cache != nil {
		for i := range mutants {
			mutant := &mutants[i]
			fileHash, err := hashFile(mutant.Site.File.Name())
			if err != nil {
				continue
			}
			key := cache.Key(mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column, TypeToUint8(mutant.Site.Node), mutant.Operator.Name(), fileHash)
			if entry, ok := cache.Get(key); ok {
				mutant.Status = entry.Status
			}
		}
	}

	var toRun []Mutant
	for _, m := range mutants {
		if m.Status == "" {
			toRun = append(toRun, m)
		}
	}
	if len(toRun) == 0 {
		if cache != nil {
			_ = cache.Save(baseDir)
		}
		return mutants, nil
	}
	mutants = toRun

	modPath := findGoMod(baseDir)

	baseDirAbs, _ := filepath.Abs(baseDir)
	baseGoMod := filepath.Join(baseDirAbs, "go.mod")
	hasOwnGoMod := fileExists(baseGoMod)

	if !hasOwnGoMod {
		return runSchemataStandalone(mutants, concurrent, cache, baseDir, tests)
	}

	moduleRoot := filepath.Dir(modPath)

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

	affectedPkgDirs := make(map[string]bool)
	for i := range mutants {
		mutant := &mutants[i]
		relFile, err := filepath.Rel(absModule, mutant.Site.File.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path for %s: %w", mutant.Site.File.Name(), err)
		}
		pkgDir := filepath.Dir(relFile)
		affectedPkgDirs[pkgDir] = true
	}

	goModSrc := filepath.Join(absModule, "go.mod")
	goModDst := filepath.Join(tempDir, "go.mod")
	if data, err := os.ReadFile(goModSrc); err == nil {
		if err := os.WriteFile(goModDst, data, filePermissions); err != nil {
			return nil, fmt.Errorf("failed to copy go.mod: %w", err)
		}
	}
	goSumSrc := filepath.Join(absModule, "go.sum")
	goSumDst := filepath.Join(tempDir, "go.sum")
	if data, err := os.ReadFile(goSumSrc); err == nil {
		_ = os.WriteFile(goSumDst, data, filePermissions)
	}

	for pkgRelDir := range affectedPkgDirs {
		srcPkgDir := filepath.Join(absModule, pkgRelDir)
		dstPkgDir := filepath.Join(tempDir, pkgRelDir)
		if err := os.MkdirAll(dstPkgDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create pkg dir %s: %w", dstPkgDir, err)
		}
		entries, err := os.ReadDir(srcPkgDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read pkg dir %s: %w", srcPkgDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			src := filepath.Join(srcPkgDir, name)
			dst := filepath.Join(dstPkgDir, name)
			data, err := os.ReadFile(src)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", src, err)
			}
			if err := os.WriteFile(dst, data, filePermissions); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", dst, err)
			}
		}
	}

	if err := RewriteImports(tempDir); err != nil {
		return nil, fmt.Errorf("rewrite imports: %w", err)
	}

	_ = MakeSelfContained(tempDir)

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

	hasNonStdlib := false
	for tempFile := range fileToMutants {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, tempFile, nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !isStdlibPackage(path) {
				hasNonStdlib = true
				break
			}
		}
		if hasNonStdlib {
			break
		}
	}
	if !hasNonStdlib {
		goModPath := filepath.Join(tempDir, "go.mod")
		modName := "gorgon-standalone"
		if data, err := os.ReadFile(goModPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					modName = strings.TrimPrefix(line, "module ")
					break
				}
			}
		}
		minimalMod := fmt.Sprintf("module %s\n\ngo 1.21\n", modName)
		_ = os.WriteFile(goModPath, []byte(minimalMod), filePermissions)
		_ = os.Remove(filepath.Join(tempDir, "go.sum"))
	}

	for tempFile, fileMutants := range fileToMutants {
		if err := ApplySchemataToFile(tempFile, fileMutants); err != nil {
			return nil, fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}

	if err := InjectSchemataHelpers(tempDir, fileToMutants); err != nil {
		return nil, err
	}

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

	if concurrent == 0 {
		concurrent = runtime.NumCPU()
	}

	type compileRequest struct {
		pkgDir string
		relPkg string
	}
	type compileResult struct {
		pkgDir string
		binary string
		err    error
	}

	compileChan := make(chan compileRequest, len(pkgToMutantIDs))
	compileResultChan := make(chan compileResult, len(pkgToMutantIDs))

	var compileGroup errgroup.Group
	compileGroup.SetLimit(concurrent)
	for pkgDir := range pkgToMutantIDs {
		relPkg, err := filepath.Rel(tempDir, pkgDir)
		if err != nil {
			close(compileChan)
			return nil, fmt.Errorf("failed to compute relative path: %w", err)
		}
		if relPkg == "." {
			relPkg = ""
		} else {
			relPkg = "./" + filepath.ToSlash(relPkg)
		}

		if strings.Contains(relPkg, "\n") || strings.Contains(relPkg, "\r") {
			close(compileChan)
			return nil, fmt.Errorf("invalid package path contains newline: %s", relPkg)
		}

		compileChan <- compileRequest{pkgDir: pkgDir, relPkg: relPkg}
	}
	close(compileChan)

	for req := range compileChan {
		req := req
		compileGroup.Go(func() error {
			testBinary := filepath.Join(req.pkgDir, "package.test")
			cmd := exec.Command("go", "test", "-c", "-o", testBinary, req.relPkg)
			cmd.Dir = tempDir
			if out, err := cmd.CombinedOutput(); err != nil {
				compileResultChan <- compileResult{pkgDir: req.pkgDir, err: fmt.Errorf("test compilation failed for %s:\n%s", req.relPkg, out)}
				return nil
			}
			compileResultChan <- compileResult{pkgDir: req.pkgDir, binary: testBinary}
			return nil
		})
	}

	type mutantResult struct {
		id     int
		status string
		err    error
	}

	resultsChan := make(chan mutantResult, len(mutants))
	testGroup, testCtx := errgroup.WithContext(ctx)
	testGroup.SetLimit(concurrent)

	var compilationErrorsMu sync.Mutex
	compilationErrors := make(map[string]error)

	var compileResultProcessor errgroup.Group

	compileResultProcessor.Go(func() error {
		for result := range compileResultChan {
			if result.err != nil {
				compilationErrorsMu.Lock()
				compilationErrors[result.pkgDir] = result.err
				compilationErrorsMu.Unlock()
				continue
			}

			if mutantIDs, ok := pkgToMutantIDs[result.pkgDir]; ok {
				pkgDir := result.pkgDir
				testBinary := result.binary
				mutantIDs := mutantIDs

				baselineDuration := 5 * time.Second
				baselineArgs := testArgs("5s", tests)
				baselineCmd := exec.CommandContext(testCtx, testBinary, baselineArgs...)
				baselineCmd.Dir = pkgDir
				baselineStart := time.Now()
				_ = baselineCmd.Run()
				baselineDuration = time.Since(baselineStart)
				if baselineDuration < 100*time.Millisecond {
					baselineDuration = 100 * time.Millisecond
				}
				timeout := time.Duration(float64(baselineDuration) * 3.0)
				if timeout > 30*time.Second {
					timeout = 30 * time.Second
				}
				timeoutStr := fmt.Sprintf("%.0fs", timeout.Seconds())

				for _, mutantID := range mutantIDs {
					mutantID := mutantID
					testGroup.Go(func() error {
						select {
						case <-testCtx.Done():
							return testCtx.Err()
						default:
						}

						cmd := exec.CommandContext(testCtx, testBinary, testArgs(timeoutStr, tests)...)
						cmd.Dir = pkgDir
						cmd.Env = append(os.Environ(), "GORGON_MUTANT_ID="+strconv.Itoa(mutantID))

						out, err := cmd.CombinedOutput()
						status := "survived"
						var errMsg error
						if err != nil {
							status = "killed"
							errMsg = fmt.Errorf("%s", out)
						}

						resultsChan <- mutantResult{id: mutantID, status: status, err: errMsg}
						return nil
					})
				}
			}
		}
		return nil
	})

	if err := compileGroup.Wait(); err != nil {
		close(compileResultChan)
		return nil, fmt.Errorf("compilation failed: %w", err)
	}
	close(compileResultChan)

	if err := compileResultProcessor.Wait(); err != nil {
		return nil, fmt.Errorf("compile result processing failed: %w", err)
	}

	if err := testGroup.Wait(); err != nil {
		return nil, fmt.Errorf("test execution failed: %w", err)
	}
	close(resultsChan)

	collected := 0
	for result := range resultsChan {
		idx := mutantIDToIndex[result.id]
		mutants[idx].Status = result.status
		mutants[idx].Error = result.err
		mutants[idx].TempDir = tempDir
		collected++
	}

	compilationErrorsMu.Lock()
	defer compilationErrorsMu.Unlock()
	if len(compilationErrors) > 0 {
		var errs []string
		for pkgDir, err := range compilationErrors {
			errs = append(errs, fmt.Sprintf("%s: %v", pkgDir, err))
		}
		return nil, fmt.Errorf("compilation failures: %s", strings.Join(errs, "; "))
	}

	if cache != nil {
		for i := range mutants {
			mutant := &mutants[i]
			if mutant.Status == "" {
				continue
			}
			fileHash, err := hashFile(mutant.Site.File.Name())
			if err != nil {
				continue
			}
			key := cache.Key(mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column, TypeToUint8(mutant.Site.Node), mutant.Operator.Name(), fileHash)
			cache.Set(key, mutant.Status)
		}
		_ = cache.Save(baseDir)
	}

	return mutants, nil
}

func runSchemataStandalone(mutants []Mutant, concurrent int, cache *cache.Cache, baseDir string, tests []string) ([]Mutant, error) {
	if concurrent == 0 {
		concurrent = runtime.NumCPU()
	}

	pkgToMutants := make(map[string][]*Mutant)
	for i := range mutants {
		mutant := &mutants[i]
		pkgDir := filepath.Dir(mutant.Site.File.Name())
		pkgToMutants[pkgDir] = append(pkgToMutants[pkgDir], mutant)
	}

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrent)

	for pkgDir, pkgMutants := range pkgToMutants {
		pkgDir := pkgDir
		pkgMutants := pkgMutants
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return processStandalonePkg(pkgDir, pkgMutants, concurrent, tests)
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if cache != nil {
		for i := range mutants {
			mutant := &mutants[i]
			if mutant.Status == "" {
				continue
			}
			fileHash, err := hashFile(mutant.Site.File.Name())
			if err != nil {
				continue
			}
			key := cache.Key(mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column, TypeToUint8(mutant.Site.Node), mutant.Operator.Name(), fileHash)
			cache.Set(key, mutant.Status)
		}
		_ = cache.Save(baseDir)
	}

	return mutants, nil
}

func processStandalonePkg(pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string) error {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return fmt.Errorf("failed to read dir %s: %w", pkgDir, err)
	}

	tempDir, err := os.MkdirTemp("", "gorgon-standalone-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var pkgName string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") && !entry.IsDir() {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.Join(pkgDir, entry.Name()), nil, parser.PackageClauseOnly)
			if err == nil && file.Name != nil {
				pkgName = file.Name.Name
			}
			break
		}
	}
	if pkgName == "" {
		pkgName = filepath.Base(pkgDir)
	}

	goMod := fmt.Sprintf("module %s\n\ngo 1.21\n", pkgName)
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), filePermissions); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || entry.IsDir() {
			continue
		}
		src := filepath.Join(pkgDir, entry.Name())
		dst := filepath.Join(tempDir, entry.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, filePermissions); err != nil {
			return fmt.Errorf("failed to write %s: %w", dst, err)
		}
	}

	tempFileToMutants := make(map[string][]*Mutant)
	for _, mutant := range pkgMutants {
		srcFile := mutant.Site.File.Name()
		baseName := filepath.Base(srcFile)
		tempFile := filepath.Join(tempDir, baseName)
		tempFileToMutants[tempFile] = append(tempFileToMutants[tempFile], mutant)
	}

	for tempFile, fileMutants := range tempFileToMutants {
		if err := ApplySchemataToFile(tempFile, fileMutants); err != nil {
			return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}

	if err := InjectSchemataHelpers(tempDir, tempFileToMutants); err != nil {
		return err
	}

	testBinary := filepath.Join(tempDir, "package.test")
	cmd := exec.Command("go", "test", "-c", "-o", testBinary, ".")
	cmd.Dir = tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		for _, mutant := range pkgMutants {
			mutant.Status = "error"
			mutant.Error = fmt.Errorf("compilation failed: %s", out)
			mutant.TempDir = tempDir
		}
		return nil
	}

	baselineCmd := exec.Command(testBinary, testArgs("5s", tests)...)
	baselineCmd.Dir = tempDir
	baselineStart := time.Now()
	_ = baselineCmd.Run()
	baselineDuration := time.Since(baselineStart)
	if baselineDuration < 100*time.Millisecond {
		baselineDuration = 100 * time.Millisecond
	}
	timeout := time.Duration(float64(baselineDuration) * 3.0)
	if timeout > 5*time.Second {
		timeout = 5 * time.Second
	}
	timeoutStr := fmt.Sprintf("%.0fs", timeout.Seconds())

	g, gCtx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrent)

	for _, mutant := range pkgMutants {
		mutant := mutant
		g.Go(func() error {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			default:
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, testBinary, testArgs(timeoutStr, tests)...)
			cmd.Dir = tempDir
			cmd.Env = append(os.Environ(), "GORGON_MUTANT_ID="+strconv.Itoa(mutant.ID))

			if out, err := cmd.CombinedOutput(); err != nil {
				mutant.Status = "killed"
				mutant.Error = fmt.Errorf("%s", out)
			} else {
				mutant.Status = "survived"
			}
			mutant.TempDir = tempDir
			return nil
		})
	}

	return g.Wait()
}

func RewriteImports(_ string) error {
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
		if !strings.Contains(content, "replace github.com/aclfe/gorgon =>") {
			content = strings.TrimSpace(content) + "\n\nreplace github.com/aclfe/gorgon => ./\n"
		}
	}

	if err := os.WriteFile(goModPath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	return nil
}

func isStdlibPackage(path string) bool {
	if path == "" {
		return false
	}
	if path[0] == '.' {
		return false
	}
	dot := strings.IndexByte(path, '.')
	slash := strings.IndexByte(path, '/')
	if dot < 0 || (slash >= 0 && slash < dot) {
		return true
	}
	return false
}

func getNodePositionForMatching(node ast.Node, fset *token.FileSet) token.Position {
	if be, ok := node.(*ast.BinaryExpr); ok {
		return fset.Position(be.OpPos)
	}
	if ids, ok := node.(*ast.IncDecStmt); ok {
		return fset.Position(ids.TokPos)
	}
	return fset.Position(node.Pos())
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
		Type   uint8
	}
	posToMutants := make(map[posKey][]schemata_nodes.MutantForSite, len(fileMutants))
	for _, mutant := range fileMutants {
		key := posKey{
			Line:   mutant.Site.Line,
			Column: mutant.Site.Column,
			Type:   TypeToUint8(mutant.Site.Node),
		}
		posToMutants[key] = append(posToMutants[key], schemata_nodes.MutantForSite{
			ID:            mutant.ID,
			Op:            mutant.Operator,
			ReturnType:    mutant.Site.ReturnType,
			EnclosingFunc: mutant.Site.EnclosingFunc,
		})
	}

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

	astutil.Apply(file, nil, func(cursor *astutil.Cursor) bool {
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

		newPos := getNodePositionForMatching(node, fset)
		key := posKey{Line: newPos.Line, Column: newPos.Column, Type: TypeToUint8(node)}
		if mutants, ok := posToMutants[key]; ok {
			returnType := ""
			if len(mutants) > 0 {
				returnType = mutants[0].ReturnType
			}
			schemata := createSchemataExpr(node, mutants, returnType, file)
			if schemata != nil && schemata != node {
				if _, isExpr := node.(ast.Expr); isExpr {
					if _, ok := schemata.(ast.Expr); ok {
						safeReplace(cursor, schemata)
					}
				} else if isValidReplacement(node, schemata) {
					safeReplace(cursor, schemata)
				}
			}
		}
		return true
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

func safeReplace(cursor *astutil.Cursor, replacement ast.Node) {
	defer func() { _ = recover() }()
	cursor.Replace(replacement)
}

func TypeToUint8(node ast.Node) uint8 {
	switch node.(type) {
	case *ast.BinaryExpr:
		return 1
	case *ast.UnaryExpr:
		return 2
	case *ast.CallExpr:
		return 3
	case *ast.Ident:
		return 4
	case *ast.CaseClause:
		return 5
	case *ast.IfStmt:
		return 6
	case *ast.ForStmt:
		return 7
	case *ast.RangeStmt:
		return 8
	case *ast.AssignStmt:
		return 9
	case *ast.IncDecStmt:
		return 10
	case *ast.DeferStmt:
		return 11
	case *ast.GoStmt:
		return 12
	case *ast.SendStmt:
		return 13
	case *ast.SwitchStmt:
		return 14
	case *ast.TypeSwitchStmt:
		return 15
	case *ast.ReturnStmt:
		return 16
	case *ast.BranchStmt:
		return 17
	case *ast.SelectStmt:
		return 18
	case *ast.CommClause:
		return 19
	case *ast.LabeledStmt:
		return 20
	case *ast.ExprStmt:
		return 21
	case *ast.DeclStmt:
		return 22
	case *ast.EmptyStmt:
		return 23
	case *ast.BlockStmt:
		return 24
	case *ast.FuncDecl:
		return 25
	case *ast.BasicLit:
		return 26
	default:
		return 0
	}
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

	typeOriginal := TypeToUint8(original)
	typeReplacement := TypeToUint8(replacement)

	if typeOriginal == typeReplacement {
		return true
	}

	validReplacements := map[uint8][]uint8{
		1:  {1, 3},
		2:  {2, 3},
		3:  {3},
		4:  {4, 3, 26},
		26: {26, 3, 4},
		5:  {5},
		6:  {6, 24},
		7:  {7, 24},
		8:  {8, 24},
		9:  {9, 24, 21},
		10: {10, 3, 24},
		11: {11, 21, 23, 24},
		12: {12, 21, 23, 24},
		13: {13, 3, 24},
		14: {14, 24},
		15: {15, 24},
		16: {16, 24, 23},
		17: {17, 21, 23, 24},
		18: {18, 24},
		19: {19, 24},
		20: {20, 24},
		21: {21, 24, 23},
		22: {22, 24},
		23: {23, 24},
		24: {24},
		25: {25},
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
