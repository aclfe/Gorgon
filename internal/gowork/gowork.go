package gowork

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Workspace represents a parsed go.work file.
type Workspace struct {
	// Root is the directory containing go.work.
	Root string
	// Modules is the absolute path of each "use" entry.
	Modules []string
}

// Find walks up from startDir looking for a go.work file.
// Returns nil if none is found (not an error — caller falls back to go.mod).
func Find(startDir string) *Workspace {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return nil
	}

	dir := abs
	for {
		candidate := filepath.Join(dir, "go.work")
		if _, err := os.Stat(candidate); err == nil {
			ws, err := parse(candidate)
			if err != nil {
				return nil
			}
			return ws
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

// ContainsPath reports whether absPath is inside any of the workspace's member modules.
func (w *Workspace) ContainsPath(absPath string) bool {
	for _, mod := range w.Modules {
		if absPath == mod ||
			strings.HasPrefix(absPath, mod+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ModuleFor returns the member module root that owns absPath,
// or "" if none of the workspace members owns the path.
func (w *Workspace) ModuleFor(absPath string) string {
	best := ""
	for _, mod := range w.Modules {
		if (absPath == mod || strings.HasPrefix(absPath, mod+string(filepath.Separator))) &&
			len(mod) > len(best) {
			best = mod
		}
	}
	return best
}

// parse reads a go.work file and returns a Workspace.
func parse(workPath string) (*Workspace, error) {
	f, err := os.Open(workPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	root := filepath.Dir(workPath)
	ws := &Workspace{Root: root}

	scanner := bufio.NewScanner(f)
	inUse := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Strip inline comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		// Block open: "use (" or "use ./path"
		if strings.HasPrefix(line, "use") {
			rest := strings.TrimSpace(line[3:])
			if rest == "(" {
				inUse = true
				continue
			}
			// Inline single use: "use ./foo"
			if rest != "" {
				ws.Modules = append(ws.Modules, absUse(root, rest))
			}
			continue
		}

		if inUse {
			if line == ")" {
				inUse = false
				continue
			}
			ws.Modules = append(ws.Modules, absUse(root, line))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ws, nil
}

func absUse(root, rel string) string {
	rel = strings.Trim(rel, `"`)
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Clean(filepath.Join(root, rel))
}
