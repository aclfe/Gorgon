package testing

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/aclfe/gorgon/internal/gowork"
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

	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(w.absModule, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file %s is outside workspace root %s", filePath, w.absModule)
	}

	w.fileRelPaths[filePath] = rel
	return rel, nil
}

func (w *ModuleWorkspace) tempPath(filePath string) (string, error) {
	rel, err := w.relPath(filePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(w.TempDir, rel), nil
}

func (w *ModuleWorkspace) tempPkgDir(filePath string) (string, error) {
	p, err := w.tempPath(filePath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

func (w *ModuleWorkspace) Cleanup() {
	if os.Getenv("GORGON_KEEP_TMPDIR") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] Keeping temp dir: %s\n", w.TempDir)
		return
	}
	_ = os.RemoveAll(w.TempDir)
}

var skippedDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
}

func (w *ModuleWorkspace) Setup(baseDir string, _ []Mutant) error {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", baseDir, err)
	}
	if info, err := os.Stat(absBase); err == nil && !info.IsDir() {
		absBase = filepath.Dir(absBase)
	}

	synthesize := false
	if ws := gowork.Find(absBase); ws != nil {
		w.goWork = ws
		w.absModule = ws.Root
	} else if modDir := FindGoModDir(absBase); modDir != "" {
		w.absModule = modDir
	} else {
		w.absModule = absBase
		synthesize = true
	}

	if err := copyTree(w.absModule, w.TempDir); err != nil {
		return fmt.Errorf("failed to copy %s into workspace: %w", w.absModule, err)
	}

	if synthesize {
		goMod := fmt.Sprintf("module %s\n\ngo %s\n", defaultModuleName, goVersion)
		if err := os.WriteFile(filepath.Join(w.TempDir, "go.mod"), []byte(goMod), filePermissions); err != nil {
			return fmt.Errorf("failed to write go.mod: %w", err)
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	type job struct{ from, to string }
	var jobs []job

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if path != src && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			if path == dst {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			return os.Symlink(link, target)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		jobs = append(jobs, job{path, target})
		return nil
	})
	if err != nil {
		return err
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(maxConcurrency() * 2)
	for _, j := range jobs {
		j := j
		g.Go(func() error {
			return copyFileWithBuffer(j.from, j.to)
		})
	}
	return g.Wait()
}

func (w *ModuleWorkspace) restoreOriginal(srcFile string) error {
	dst, err := w.tempPath(srcFile)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return fmt.Errorf("re-read %s: %w", srcFile, err)
	}
	return os.WriteFile(dst, data, filePermissions)
}
