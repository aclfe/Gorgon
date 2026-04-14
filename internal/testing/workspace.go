package testing

import (
	"context"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

func maxConcurrency() int { return runtime.NumCPU() }

type ModuleWorkspace struct {
	TempDir      string
	absModule    string
	fileRelPaths map[string]string
	mu           sync.Mutex
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
	w.mu.Lock()
	defer w.mu.Unlock()
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

	mutatedPaths := make(map[string]bool, len(mutants))
	for i := range mutants {
		if mutants[i].Site.File != nil {
			mutatedPaths[mutants[i].Site.File.Name()] = true
		}
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrency())

	g.Go(func() error {
		if err := copyFileWithBuffer(filepath.Join(absModule, "go.mod"), filepath.Join(w.TempDir, "go.mod")); err != nil {
			return fmt.Errorf("failed to copy go.mod: %w", err)
		}
		if data, err := os.ReadFile(filepath.Join(absModule, "go.sum")); err == nil {
			if err := os.WriteFile(filepath.Join(w.TempDir, "go.sum"), data, filePermissions); err != nil {
				return fmt.Errorf("failed to copy go.sum: %w", err)
			}
		}
		return nil
	})

	allPkgs, err := collectAllPackages(absModule)
	if err != nil {
		return fmt.Errorf("failed to collect packages: %w", err)
	}

	for pkgRelDir := range allPkgs {
		pkgRelDir := pkgRelDir
		g.Go(func() error {
			return w.copyPackage(absModule, pkgRelDir, mutatedPaths)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

func (w *ModuleWorkspace) applySchemata(mutants []Mutant) (map[string][]*Mutant, bool, error) {
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrency())

	astToMutants := make(map[*ast.File][]*Mutant, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		if m.Site.FileAST != nil {
			astToMutants[m.Site.FileAST] = append(astToMutants[m.Site.FileAST], m)
		}
	}

	hasNonStdlib := false
	for astFile := range astToMutants {
		for _, imp := range astFile.Imports {
			if !isStdlib(strings.Trim(imp.Path.Value, `"`)) {
				hasNonStdlib = true
				break
			}
		}
		if hasNonStdlib {
			break
		}
	}

	sourceCache := make(map[string][]byte)
	var cacheMu sync.Mutex

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
		entryAST := entry.astFile
		entryMutants := entry.mutants

		origPath := entryMutants[0].Site.File.Name()
		if origPath == "" {
			continue
		}

		g.Go(func() error {

			cacheMu.Lock()
			src, cached := sourceCache[origPath]
			cacheMu.Unlock()

			if !cached {
				var err error
				src, err = os.ReadFile(origPath)
				if err != nil {
					return fmt.Errorf("failed to read %s: %w", origPath, err)
				}
				cacheMu.Lock()
				sourceCache[origPath] = src
				cacheMu.Unlock()
			}

			rel, err := w.relPath(origPath)
			if err != nil {
				return fmt.Errorf("failed to compute rel path: %w", err)
			}
			tempFile := filepath.Join(w.TempDir, rel)

			posMap, err := ApplySchemataToAST(entryAST, entryMutants[0].Site.Fset, tempFile, src, entryMutants)
			if err != nil {
				return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
			}
			for _, m := range entryMutants {
				if pm, ok := posMap[m.ID]; ok {
					m.TempLine = pm.TempLine
					m.TempCol = pm.TempCol
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, false, err
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
			return nil, false, fmt.Errorf("failed to compute rel path: %w", err)
		}
		tempFile := filepath.Join(w.TempDir, rel)
		fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
	}

	if err := InjectSchemataHelpers(fileToMutants); err != nil {
		return nil, false, err
	}

	return fileToMutants, hasNonStdlib, nil
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

func (w *ModuleWorkspace) simplifyGoMod(hasNonStdlib bool) {
	if hasNonStdlib {
		return
	}

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

func (w *ModuleWorkspace) copyPackage(absModule, pkgRelDir string, mutatedPaths map[string]bool) error {
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
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {

			if hasGoFiles(src) {
				if err := copyDirContents(src, dst, mutatedPaths); err != nil {
					return fmt.Errorf("failed to copy dir %s: %w", entry.Name(), err)
				}
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		if mutatedPaths[src] {
			continue
		}

		if err := copyFileWithBuffer(src, dst); err != nil {
			return fmt.Errorf("failed to copy %s: %w", src, err)
		}
	}
	return nil
}

func collectAllPackages(absModule string) (map[string]bool, error) {
	pkgs := make(map[string]bool)

	err := filepath.Walk(absModule, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == ".git" || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			dir := filepath.Dir(path)
			relDir, err := filepath.Rel(absModule, dir)
			if err != nil {
				return nil
			}
			pkgs[relDir] = true
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return pkgs, nil
}
