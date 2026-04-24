package testing

import (
	"bufio"
	"bytes"
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

	"github.com/aclfe/gorgon/internal/gowork"
	"github.com/aclfe/gorgon/internal/logger"
)

func maxConcurrency() int { return runtime.NumCPU() }

type ModuleWorkspace struct {
	TempDir      string
	absModule    string
	goWork       *gowork.Workspace
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

	// In workspace mode, find which member module owns this file
	// so the relative path never escapes the temp dir via "..".
	root := w.absModule
	if w.goWork != nil {
		if owner := w.goWork.ModuleFor(filePath); owner != "" {
			root = w.goWork.Root
		}
	}

	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", err
	}
	// Safety: if we still get a "../" path the file is outside every known root.
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("file %s is outside workspace root %s", filePath, root)
	}

	w.fileRelPaths[filePath] = rel
	return rel, nil
}

func (w *ModuleWorkspace) Cleanup() {
	_ = os.RemoveAll(w.TempDir)
}

func (w *ModuleWorkspace) Setup(baseDir string, mutants []Mutant) error {
	// Detect go.work first; fall back to go.mod if absent.
	ws := gowork.Find(baseDir)
	w.goWork = ws

	var moduleRoots []string

	if ws != nil {
		w.absModule = ws.Root
		moduleRoots = ws.Modules
	} else {
		modDir := FindGoModDir(baseDir)
		if modDir == "" {
			return fmt.Errorf("no go.mod found in %s or any parent directory", baseDir)
		}
		abs, err := filepath.Abs(modDir)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for module root: %w", err)
		}
		w.absModule = abs
		moduleRoots = []string{abs}
	}

	mutatedPaths := make(map[string]bool, len(mutants))
	for i := range mutants {
		if mutants[i].Site.File != nil {
			mutatedPaths[mutants[i].Site.File.Name()] = true
		}
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrency())

	// Copy go.work + go.work.sum when present.
	if ws != nil {
		g.Go(func() error {
			return copyGoWork(ws, w.TempDir)
		})
	}

	// Copy go.mod + go.sum for every member module.
	for _, modRoot := range moduleRoots {
		modRoot := modRoot
		g.Go(func() error {
			rel, err := filepath.Rel(w.absModule, modRoot)
			if err != nil {
				rel = filepath.Base(modRoot)
			}
			dstRoot := filepath.Join(w.TempDir, rel)
			if dstRoot == filepath.Join(w.TempDir, ".") {
				dstRoot = w.TempDir
			}
			if err := os.MkdirAll(dstRoot, 0o755); err != nil {
				return err
			}
			if err := copyFileWithBuffer(
				filepath.Join(modRoot, "go.mod"),
				filepath.Join(dstRoot, "go.mod"),
			); err != nil {
				return fmt.Errorf("failed to copy go.mod from %s: %w", modRoot, err)
			}
			if data, err := os.ReadFile(filepath.Join(modRoot, "go.sum")); err == nil {
				_ = os.WriteFile(filepath.Join(dstRoot, "go.sum"), data, filePermissions)
			}
			return nil
		})
	}

	// Collect and copy packages across all member modules.
	for _, modRoot := range moduleRoots {
		modRoot := modRoot
		allPkgs, err := collectAllPackages(modRoot)
		if err != nil {
			return fmt.Errorf("failed to collect packages in %s: %w", modRoot, err)
		}
		for pkgRelDir := range allPkgs {
			pkgRelDir := pkgRelDir
			g.Go(func() error {
				return w.copyPackageFromModule(modRoot, pkgRelDir, mutatedPaths)
			})
		}
	}

	return g.Wait()
}

func (w *ModuleWorkspace) applySchemata(mutants []Mutant, log *logger.Logger) (map[string][]*Mutant, bool, error) {
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

	if err := InjectSchemataHelpers(fileToMutants, log); err != nil {
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
	// Never strip go.mod content when a workspace is active —
	// member modules may reference each other through go.work.
	if hasNonStdlib || w.goWork != nil {
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

// copyPackageFromModule is like copyPackage but the relative dest path is
// computed from modRoot, then placed under w.TempDir at the same relative
// position it holds within w.absModule (the workspace root).
func (w *ModuleWorkspace) copyPackageFromModule(modRoot, pkgRelDir string, mutatedPaths map[string]bool) error {
	srcDir := filepath.Join(modRoot, pkgRelDir)

	// Destination is relative to workspace root (w.absModule), not modRoot.
	modRelToWorkspace, err := filepath.Rel(w.absModule, modRoot)
	if err != nil {
		modRelToWorkspace = filepath.Base(modRoot)
	}
	dstDir := filepath.Join(w.TempDir, modRelToWorkspace, pkgRelDir)
	if dstDir == filepath.Join(w.TempDir, ".", pkgRelDir) {
		// modRoot IS the workspace root (single-module case)
		dstDir = filepath.Join(w.TempDir, pkgRelDir)
	}

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

// copyGoWork writes go.work and go.work.sum into the temp dir,
// rewriting each "use" path to point at the temp subdirectory.
func copyGoWork(ws *gowork.Workspace, tempDir string) error {
	srcPath := filepath.Join(ws.Root, "go.work")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read go.work: %w", err)
	}

	// Rewrite "use" lines so they point into tempDir.
	var out strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "use ") && !strings.HasSuffix(trimmed, "(") {
			// Inline single use: rewrite the path.
			rel := strings.TrimSpace(trimmed[4:])
			rel = strings.Trim(rel, `"`)
			abs := filepath.Clean(filepath.Join(ws.Root, rel))
			newRel, err := filepath.Rel(ws.Root, abs)
			if err != nil {
				newRel = rel
			}
			// In tempDir the member module lives at the same relative path.
			out.WriteString(fmt.Sprintf("use %s\n", newRel))
			continue
		}
		out.WriteString(line + "\n")
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	dst := filepath.Join(tempDir, "go.work")
	if err := os.WriteFile(dst, []byte(out.String()), filePermissions); err != nil {
		return fmt.Errorf("failed to write go.work: %w", err)
	}

	// Copy go.work.sum if present.
	if sumData, err := os.ReadFile(filepath.Join(ws.Root, "go.work.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(tempDir, "go.work.sum"), sumData, filePermissions)
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

func (w *ModuleWorkspace) copyExternalSuites(absModule string, suitePaths []string, log *logger.Logger) error {
	for _, relPath := range suitePaths {
		dirs, err := expandGlobPath(absModule, relPath)
		if err != nil {
			continue
		}
		for _, dir := range dirs {
			rel, err := filepath.Rel(absModule, dir)
			if err != nil {
				continue
			}
			dst := filepath.Join(w.TempDir, rel)
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			entries, _ := os.ReadDir(dir)
			copiedCount := 0
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
					continue
				}
				if err := copyFileWithBuffer(
					filepath.Join(dir, e.Name()),
					filepath.Join(dst, e.Name()),
				); err != nil {
					return err
				}
				copiedCount++
			}
			log.Debug("[EXTERNAL] Copied %d files from %s to %s", copiedCount, dir, dst)
		}
	}
	return nil
}

func expandGlobPath(absModule, pattern string) ([]string, error) {
	clean := strings.TrimPrefix(pattern, "./")
	isRecursive := strings.HasSuffix(clean, "/...")
	if isRecursive {
		clean = strings.TrimSuffix(clean, "/...")
	}
	root := filepath.Join(absModule, clean)

	if !isRecursive {
		return []string{root}, nil
	}

	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		entries, _ := os.ReadDir(path)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), "_test.go") {
				dirs = append(dirs, path)
				return nil
			}
		}
		return nil
	})
	return dirs, nil
}
