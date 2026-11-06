package testing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/core/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/config"
)

type fingerprinter struct {
	moduleRoot string
	buildTags  []string
	configHash string

	mu    sync.Mutex
	byPkg map[string]string
	byDir map[string]string
}

func newFingerprinter(projectRoot string, buildTags []string, configHash string) *fingerprinter {
	root := projectRoot
	if abs, err := filepath.Abs(projectRoot); err == nil {
		root = abs
	}
	if mod := FindGoModDir(root); mod != "" {
		root = mod
	}
	return &fingerprinter{
		moduleRoot: root,
		buildTags:  buildTags,
		configHash: configHash,
		byPkg:      make(map[string]string),
		byDir:      make(map[string]string),
	}
}

func cacheConfigFingerprint(testsByPkg map[string][]string, unitTestsEnabled bool, ext config.ExternalSuitesConfig) string {
	h := sha256.New()
	fmt.Fprintf(h, "unit=%v\n", unitTestsEnabled)
	keys := make([]string, 0, len(testsByPkg))
	for k := range testsByPkg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		tests := append([]string(nil), testsByPkg[k]...)
		sort.Strings(tests)
		fmt.Fprintf(h, "tests %s=%s\n", k, strings.Join(tests, ","))
	}
	fmt.Fprintf(h, "external=%v mode=%s\n", ext.Enabled, ext.RunMode)
	if ext.Enabled {
		for _, s := range ext.Suites {
			fmt.Fprintf(h, "suite %s paths=%s tags=%s\n", s.Name, strings.Join(s.Paths, ","), strings.Join(s.Tags, ","))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (f *fingerprinter) packageFingerprint(pkgDir string) string {
	f.mu.Lock()
	if fp, ok := f.byPkg[pkgDir]; ok {
		f.mu.Unlock()
		return fp
	}
	f.mu.Unlock()

	dirs := f.inModuleDeps(pkgDir)
	h := sha256.New()
	fmt.Fprintf(h, "schema=%d\nconfig=%s\ntags=%s\n", cache.SchemaVersion, f.configHash, strings.Join(f.buildTags, ","))
	for _, name := range []string{"go.mod", "go.sum", "go.work", "go.work.sum"} {
		if data, err := os.ReadFile(filepath.Join(f.moduleRoot, name)); err == nil {
			fmt.Fprintf(h, "%s:%x\n", name, sha256.Sum256(data))
		}
	}
	for _, d := range dirs {
		fmt.Fprintf(h, "dir %s:%s\n", d, f.dirHash(d))
	}
	fp := hex.EncodeToString(h.Sum(nil))

	f.mu.Lock()
	f.byPkg[pkgDir] = fp
	f.mu.Unlock()
	return fp
}

func (f *fingerprinter) inModuleDeps(pkgDir string) []string {
	args := []string{"list", "-deps", "-test", "-f", "{{.Dir}}"}
	if len(f.buildTags) > 0 {
		args = append(args, "-tags", strings.Join(f.buildTags, ","))
	}
	args = append(args, ".")
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Dir = pkgDir
	out, err := cmd.Output()

	set := map[string]bool{pkgDir: true}
	if err != nil {
		_ = filepath.WalkDir(f.moduleRoot, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if skippedDirs[d.Name()] || d.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".go") {
				set[filepath.Dir(p)] = true
			}
			return nil
		})
	} else {
		prefix := f.moduleRoot + string(filepath.Separator)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == f.moduleRoot || strings.HasPrefix(line, prefix) {
				set[line] = true
			}
		}
	}
	dirs := make([]string, 0, len(set))
	for d := range set {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

func (f *fingerprinter) dirHash(dir string) string {
	f.mu.Lock()
	if h, ok := f.byDir[dir]; ok {
		f.mu.Unlock()
		return h
	}
	f.mu.Unlock()

	h := sha256.New()
	hashFileInto := func(p string) {
		data, err := os.ReadFile(p)
		if err != nil {
			return
		}
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(h, "%s:%x\n", rel, sha256.Sum256(data))
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.Type().IsRegular() {
			hashFileInto(p)
			continue
		}
		if e.IsDir() && e.Name() == "testdata" {
			_ = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
				if err == nil && d.Type().IsRegular() {
					hashFileInto(q)
				}
				return nil
			})
		}
	}
	sum := hex.EncodeToString(h.Sum(nil))
	f.mu.Lock()
	f.byDir[dir] = sum
	f.mu.Unlock()
	return sum
}

func (f *fingerprinter) mutantKey(c *cache.Cache, m *Mutant) string {
	file := m.Site.File.Name()
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	fp := f.packageFingerprint(filepath.Dir(abs))
	return c.Key(abs, m.Site.Offset, m.Site.EndOffset,
		schemata_nodes.NodeTypeToUint8(m.Site.Node), m.Operator.Name(), fp)
}

func cacheable(status string) bool {
	switch status {
	case StatusKilled, StatusSurvived, StatusTimeout:
		return true
	}
	return false
}

func (r *run) applyCache(c *cache.Cache, fp *fingerprinter) {
	hits := 0
	for _, m := range r.pending() {
		if m.Site.File == nil || m.Operator == nil {
			continue
		}
		entry, ok := c.Get(fp.mutantKey(c, m))
		if !ok || !cacheable(entry.Status) {
			continue
		}
		m.Status = entry.Status
		m.KilledBy = entry.KilledBy
		m.KillOutput = "(cached result)"
		hits++
	}
	if hits > 0 {
		r.log.Info("[CACHE] reused %d verdict(s)", hits)
	}
}

func (r *run) saveCache(c *cache.Cache, fp *fingerprinter, baseDir string) {
	if c == nil {
		return
	}
	for i := range r.all {
		m := &r.all[i]
		if !cacheable(m.Status) || m.Site.File == nil || m.Operator == nil {
			continue
		}
		c.SetEntry(fp.mutantKey(c, m), cache.Entry{Status: m.Status, KilledBy: m.KilledBy})
	}
	if err := c.Save(baseDir); err != nil {
		r.log.Warn("failed to save cache: %v", err)
	}
}
