package testing

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)


type ModuleWorkspace struct {
	TempDir   string
	absModule string
	fileRelPaths map[string]string
}


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


func (w *ModuleWorkspace) Cleanup() {
	_ = os.RemoveAll(w.TempDir)
}


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


	if err := copyFileWithBuffer(filepath.Join(absModule, "go.mod"), filepath.Join(w.TempDir, "go.mod")); err != nil {
		return fmt.Errorf("failed to copy go.mod: %w", err)
	}
	if data, err := os.ReadFile(filepath.Join(absModule, "go.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(w.TempDir, "go.sum"), data, filePermissions)
	}


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



func (w *ModuleWorkspace) applySchemata(mutants []Mutant) (map[string][]*Mutant, error) {

	astToMutants := make(map[*ast.File][]*Mutant, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		if m.Site.FileAST != nil {
			astToMutants[m.Site.FileAST] = append(astToMutants[m.Site.FileAST], m)
		}
	}

	sourceCache := make(map[string][]byte)

	type astEntry struct {
		astFile *ast.File
		mutants []*Mutant
	}
	sortedASTs := make([]astEntry, 0, len(astToMutants))
	for astFile, fileMutants := range astToMutants {
		sortedASTs = append(sortedASTs, astEntry{astFile, fileMutants})
	}
	sort.Slice(sortedASTs, func(i, j int) bool {
		if len(sortedASTs[i].mutants) == 0 || len(sortedASTs[j].mutants) == 0 {
			return false
		}
		return sortedASTs[i].mutants[0].Site.File.Name() < sortedASTs[j].mutants[0].Site.File.Name()
	})

	for _, entry := range sortedASTs {
		astFile := entry.astFile
		fileMutants := entry.mutants

		origPath := fileMutants[0].Site.File.Name()
		if origPath == "" {
			continue
		}


		if _, ok := sourceCache[origPath]; !ok {
			if src, err := os.ReadFile(origPath); err == nil {
				sourceCache[origPath] = src
			}
		}

		rel, err := w.relPath(origPath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path: %w", err)
		}
		tempFile := filepath.Join(w.TempDir, rel)


		src := sourceCache[origPath]
		if err := ApplySchemataToAST(astFile, fileMutants[0].Site.Fset, tempFile, src, fileMutants); err != nil {
			return nil, fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}

	
	fileToMutants := make(map[string][]*Mutant, len(mutants))
	
	sortedMutants := make([]*Mutant, len(mutants))
	for i := range mutants {
		sortedMutants[i] = &mutants[i]
	}
	sort.Slice(sortedMutants, func(i, j int) bool {
		return sortedMutants[i].ID < sortedMutants[j].ID
	})

	for _, m := range sortedMutants {
		rel, err := w.relPath(m.Site.File.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to compute rel path: %w", err)
		}
		tempFile := filepath.Join(w.TempDir, rel)
		fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
	}

	if err := InjectSchemataHelpers(w.TempDir, fileToMutants); err != nil {
		return nil, err
	}

	return fileToMutants, nil
}


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


func (w *ModuleWorkspace) simplifyGoMod(fileToMutants map[string][]*Mutant) {
	
	files := make([]string, 0, len(fileToMutants))
	for tempFile := range fileToMutants {
		files = append(files, tempFile)
	}
	sort.Strings(files)

	hasNonStdlib := false
	for _, tempFile := range files {
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
