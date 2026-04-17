package subconfig

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

const SubConfigFilename = "gorgon.yml"

// entry is a discovered sub-config and the absolute directory it governs.
type entry struct {
	dir    string
	config *config.SubConfig
}

// Resolver discovers and resolves per-directory config overrides.
type Resolver struct {
	projectRoot string
	entries     []entry // sorted shallowest→deepest for chain resolution
}

// Discover walks projectRoot finding all gorgon.yml files in subdirectories.
// rootConfigPath is excluded so we don't re-process the root config as a sub-config.
func Discover(projectRoot, rootConfigPath string) (*Resolver, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	absRootConfig := ""
	if rootConfigPath != "" {
		absRootConfig, _ = filepath.Abs(rootConfigPath)
	}

	r := &Resolver{projectRoot: absRoot}

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
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
		if filepath.Base(path) != SubConfigFilename {
			return nil
		}
		absPath, _ := filepath.Abs(path)
		// Skip the root config itself
		if absPath == absRootConfig {
			return nil
		}
		// Also skip a gorgon.yml sitting directly at the project root
		// if no explicit -config was given (same file, different invocation)
		if filepath.Dir(absPath) == absRoot && absRootConfig == "" {
			return nil
		}

		sc, err := config.LoadSubConfig(absPath)
		if err != nil {
			return err // surface parse errors, don't silently swallow
		}
		r.entries = append(r.entries, entry{
			dir:    filepath.Dir(absPath),
			config: sc,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort shallowest first so chain resolution can iterate in order
	sort.Slice(r.entries, func(i, j int) bool {
		return len(r.entries[i].dir) < len(r.entries[j].dir)
	})

	return r, nil
}

// chain returns all entries that are ancestors-or-self of the given file path,
// ordered shallowest→deepest. This is the resolution chain for that file.
func (r *Resolver) chain(filePath string) []*config.SubConfig {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return nil
	}
	fileDir := filepath.Dir(absFile)

	var chain []*config.SubConfig
	for i := range r.entries {
		e := &r.entries[i]
		// Matches if the entry dir IS the file's dir, or is an ancestor
		if fileDir == e.dir ||
			strings.HasPrefix(fileDir+string(filepath.Separator), e.dir+string(filepath.Separator)) {
			chain = append(chain, e.config)
		}
	}
	return chain // already sorted shallowest→deepest from Discover
}

// EffectiveOperators returns the operator list that applies to filePath.
// Replace semantics: deepest sub-config with a non-empty Operators list wins.
// Falls back to base if no sub-config specifies operators.
func (r *Resolver) EffectiveOperators(filePath string, base []mutator.Operator, allOps []mutator.Operator) []mutator.Operator {
	chain := r.chain(filePath)
	if len(chain) == 0 {
		return base
	}

	// Walk deepest→shallowest; first non-empty Operators list wins
	for i := len(chain) - 1; i >= 0; i-- {
		sc := chain[i]
		if len(sc.Operators) == 0 {
			continue
		}
		// Resolve operator names to actual Operator instances
		resolved := resolveOperatorNames(sc.Operators, allOps)
		if len(resolved) > 0 {
			return resolved
		}
	}
	return base
}

// EffectiveThreshold returns the threshold for filePath.
// Replace semantics: deepest sub-config with a non-nil Threshold wins.
func (r *Resolver) EffectiveThreshold(filePath string, rootThreshold float64) float64 {
	chain := r.chain(filePath)
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Threshold != nil {
			return *chain[i].Threshold
		}
	}
	return rootThreshold
}

// EffectiveFilters returns the merged exclude/include/skip/skip_func
// accumulated across the entire chain for filePath.
func (r *Resolver) EffectiveFilters(filePath string, root *config.Config) (exclude, include, skip, skipFunc []string) {
	exclude = append(exclude, root.Exclude...)
	include = append(include, root.Include...)
	skip = append(skip, root.Skip...)
	skipFunc = append(skipFunc, root.SkipFunc...)

	for _, sc := range r.chain(filePath) {
		exclude = append(exclude, sc.Exclude...)
		include = append(include, sc.Include...)
		skip = append(skip, sc.Skip...)
		skipFunc = append(skipFunc, sc.SkipFunc...)
	}
	return
}

// EffectiveSuppress returns merged suppressions across the chain.
func (r *Resolver) EffectiveSuppress(filePath string, root []config.SuppressEntry) []config.SuppressEntry {
	result := append([]config.SuppressEntry{}, root...)
	for _, sc := range r.chain(filePath) {
		result = append(result, sc.Suppress...)
	}
	return result
}

// EffectiveDirRules returns merged dir_rules across the chain.
func (r *Resolver) EffectiveDirRules(filePath string, root []config.DirOperatorRule) []config.DirOperatorRule {
	result := append([]config.DirOperatorRule{}, root...)
	for _, sc := range r.chain(filePath) {
		result = append(result, sc.DirRules...)
	}
	return result
}

// EffectiveTests returns the test list for filePath.
// Replace semantics: deepest sub-config with a non-empty Tests list wins.
func (r *Resolver) EffectiveTests(filePath string, rootTests []string) []string {
	chain := r.chain(filePath)
	for i := len(chain) - 1; i >= 0; i-- {
		if len(chain[i].Tests) > 0 {
			return chain[i].Tests
		}
	}
	return rootTests
}

// HasAnyOverrides returns false when no sub-configs were discovered,
// letting callers skip resolution overhead entirely.
func (r *Resolver) HasAnyOverrides() bool {
	return len(r.entries) > 0
}

// Entries returns the number of discovered sub-configs.
func (r *Resolver) Entries() int {
	return len(r.entries)
}

func resolveOperatorNames(names []string, all []mutator.Operator) []mutator.Operator {
	index := make(map[string]mutator.Operator, len(all))
	for _, op := range all {
		index[op.Name()] = op
	}
	var out []mutator.Operator
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "all" {
			return all
		}
		if op, ok := index[name]; ok {
			out = append(out, op)
		}
	}
	return out
}
