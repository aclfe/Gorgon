package testing

import (
	"fmt"
<<<<<<< HEAD
=======
	"go/ast"
>>>>>>> 5607fd5 (fixing relative path and example)
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ModuleWorkspace manages the temp directory for mutation testing of a Go module.
type ModuleWorkspace struct {
	TempDir   string
	absModule string
	// fileRelPaths caches relative paths from absModule to each unique source file,
	// avoiding repeated filepath.Rel calls across setup/applySchemata/buildPkgMap.
	fileRelPaths map[string]string
}

// NewModuleWorkspace creates a temp directory for mutation testing.
func NewModuleWorkspace() (*ModuleWorkspace, error) {
	tempDir, err := os.MkdirTemp("", "gorgon-schemata-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	return &ModuleWorkspace{
		TempDir:      tempDir,
		fileRelPaths: make(map[string]string),
	}, nil
}

// relPath returns the cached relative path from absModule to filePath,
// computing and caching it if not already present.
func (w *ModuleWorkspace) relPath(filePath string) (string, error) {
	if rel, ok := w.fileRelPaths[filePath]; ok {
		return rel, nil
	}
	rel, err := filepath.Rel(w.absModule, filePath)
	if err != nil {
		return "", err
	}
	w.fileRelPaths[filePath] = rel
	return rel, nil
}

// Cleanup removes the temporary directory.
func (w *ModuleWorkspace) Cleanup() {
	_ = os.RemoveAll(w.TempDir)
}

// setup copies the module and affected packages to the temp directory.
func (w *ModuleWorkspace) setup(baseDir string, mutants []Mutant) error {
	modDir := FindGoModDir(baseDir)
	if modDir == "" {
		return fmt.Errorf("no go.mod found in %s or any parent directory", baseDir)
	}

	absModule, err := filepath.Abs(modDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for module root: %w", err)
	}
	w.absModule = absModule

	// Copy go.mod and go.sum
	if err := copyFileWithBuffer(filepath.Join(absModule, "go.mod"), filepath.Join(w.TempDir, "go.mod")); err != nil {
		return fmt.Errorf("failed to copy go.mod: %w", err)
	}
	if data, err := os.ReadFile(filepath.Join(absModule, "go.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(w.TempDir, "go.sum"), data, filePermissions)
	}

	// Determine affected packages and copy them, caching relative paths
	affected := make(map[string]bool)
	for i := range mutants {
		rel, err := w.relPath(mutants[i].Site.File.Name())
		if err != nil {
			return fmt.Errorf("failed to compute rel path: %w", err)
		}
		affected[filepath.Dir(rel)] = true
	}

	for pkgRelDir := range affected {
		if err := w.copyPackage(absModule, pkgRelDir); err != nil {
			return err
		}
	}
	return nil
}

<<<<<<< HEAD
// applySchemata applies mutations to files and injects the helper file.
func (w *ModuleWorkspace) applySchemata(mutants []Mutant) (map[string][]*Mutant, error) {
=======
// applySchemata applies mutations to files using pre-parsed AST from Phase 1.
// This avoids re-parsing files in Phase 2.
func (w *ModuleWorkspace) applySchemata(mutants []Mutant) (map[string][]*Mutant, error) {
	// Group mutants by their pre-parsed AST (from Phase 1) to avoid re-parsing
	astToMutants := make(map[*ast.File][]*Mutant, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		if m.Site.FileAST != nil {
			astToMutants[m.Site.FileAST] = append(astToMutants[m.Site.FileAST], m)
		}
	}

	// Read source files once for fallback when format fails
	sourceCache := make(map[string][]byte)

	// Apply schemata using pre-parsed ASTs
	for astFile, fileMutants := range astToMutants {
		// Get original file path from any mutant's Site
		origPath := fileMutants[0].Site.File.Name()
		if origPath == "" {
			continue
		}

		// Cache source if not already done
		if _, ok := sourceCache[origPath]; !ok {
			if src, err := os.ReadFile(origPath); err == nil {
				sourceCache[origPath] = src
			}
		}

		// Compute temp file path
		rel, err := w.relPath(origPath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path: %w", err)
		}
		tempFile := filepath.Join(w.TempDir, rel)

		// Use pre-parsed AST - no re-parsing needed
		src := sourceCache[origPath]
		if err := ApplySchemataToAST(astFile, fileMutants[0].Site.Fset, tempFile, src, fileMutants); err != nil {
			return nil, fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}

	// Build fileToMutants map for InjectSchemataHelpers
>>>>>>> 5607fd5 (fixing relative path and example)
	fileToMutants := make(map[string][]*Mutant, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		rel, err := w.relPath(m.Site.File.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path: %w", err)
		}
		tempFile := filepath.Join(w.TempDir, rel)
		fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
	}

<<<<<<< HEAD
	for tempFile, fileMutants := range fileToMutants {
		if err := ApplySchemataToFile(tempFile, fileMutants); err != nil {
			return nil, fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}

=======
>>>>>>> 5607fd5 (fixing relative path and example)
	if err := InjectSchemataHelpers(w.TempDir, fileToMutants); err != nil {
		return nil, err
	}

	return fileToMutants, nil
}

// buildPkgMap builds the package-to-mutant-IDs mapping and mutant ID to index mapping.
func (w *ModuleWorkspace) buildPkgMap(mutants []Mutant) (map[string][]int, map[int]int, error) {
	pkgToIDs := make(map[string][]int, len(mutants))
	idToIndex := make(map[int]int, len(mutants))

	for idx := range mutants {
		m := &mutants[idx]
		rel, err := w.relPath(m.Site.File.Name())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compute rel path: %w", err)
		}
		pkgDir := filepath.Join(w.TempDir, filepath.Dir(rel))
		pkgToIDs[pkgDir] = append(pkgToIDs[pkgDir], m.ID)
		idToIndex[m.ID] = idx
	}

	return pkgToIDs, idToIndex, nil
}

// simplifyGoMod removes external dependencies if only stdlib is used.
func (w *ModuleWorkspace) simplifyGoMod(fileToMutants map[string][]*Mutant) {
	hasNonStdlib := false
	for tempFile := range fileToMutants {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, tempFile, nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range f.Imports {
			if !isStdlib(strings.Trim(imp.Path.Value, `"`)) {
				hasNonStdlib = true
				break
			}
		}
		if hasNonStdlib {
			break
		}
	}

	if !hasNonStdlib {
		goModPath := filepath.Join(w.TempDir, "go.mod")
		modName := defaultModuleName
		if data, err := os.ReadFile(goModPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					modName = strings.TrimPrefix(line, "module ")
					break
				}
			}
		}
		_ = os.WriteFile(goModPath, []byte(fmt.Sprintf("module %s\n\ngo %s\n", modName, goVersion)), filePermissions)
		_ = os.Remove(filepath.Join(w.TempDir, "go.sum"))
	}
}

func (w *ModuleWorkspace) copyPackage(absModule, pkgRelDir string) error {
	srcDir := filepath.Join(absModule, pkgRelDir)
	dstDir := filepath.Join(w.TempDir, pkgRelDir)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("failed to create pkg dir %s: %w", dstDir, err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read pkg dir %s: %w", srcDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())
		if err := copyFileWithBuffer(src, dst); err != nil {
			return fmt.Errorf("failed to copy %s: %w", src, err)
		}
	}
	return nil
}
