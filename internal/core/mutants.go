package testing

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/core/schemata_nodes"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// Mutant status constants — single source of truth.
const (
	StatusKilled       = "killed"
	StatusSurvived     = "survived"
	StatusUntested     = "untested"
	StatusInvalid      = "invalid"
	StatusTimeout      = "timeout"
	StatusError        = "error"
	StatusCompileError = "error" // compile errors share the "error" status; distinguished by KilledBy == "(compiler)"
)

type Mutant struct {
	ID           int
	Site         engine.Site
	Operator     mutator.Operator
	TempDir      string
	TempLine     int
	TempCol      int
	Status       string
	Error        error
	KilledBy     string
	KillDuration time.Duration
	KillOutput   string
	ErrorReason  string
}

func effectiveOperators(filePath, projectRoot string, base []mutator.Operator, rules []config.DirOperatorRule, log *logger.Logger) []mutator.Operator {
	if len(rules) == 0 {
		return base
	}

	rel, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return base
	}

	// Find longest-prefix (most specific) matching rule
	bestLen := -1
	var best *config.DirOperatorRule
	for i := range rules {
		r := &rules[i]
		dir := filepath.Clean(r.Dir)
		if rel == dir || strings.HasPrefix(rel, dir+string(filepath.Separator)) {
			if len(dir) > bestLen {
				bestLen = len(dir)
				best = r
			}
		}
	}

	if best == nil {
		return base
	}

	// Whitelist takes priority
	if len(best.Whitelist) > 0 {
		allow := make(map[string]bool, len(best.Whitelist))
		for _, op := range best.Whitelist {
			allow[strings.TrimSpace(op)] = true
		}
		var out []mutator.Operator
		for _, op := range base {
			if allow[op.Name()] {
				out = append(out, op)
			}
		}
		return out
	}

	if len(best.Blacklist) > 0 {
		// Check for "all" shorthand
		for _, op := range best.Blacklist {
			if strings.TrimSpace(op) == "all" {
				if log != nil && log.IsDebug() {
					log.Debug("Dir rule %s: blacklist all operators for %s", best.Dir, rel)
				}
				return nil
			}
		}
		deny := make(map[string]bool, len(best.Blacklist))
		for _, op := range best.Blacklist {
			deny[strings.TrimSpace(op)] = true
		}
		var out []mutator.Operator
		for _, op := range base {
			if !deny[op.Name()] {
				out = append(out, op)
			}
		}
		if log != nil && log.IsDebug() {
			log.Debug("Dir rule %s: blacklist %d operators for %s", best.Dir, len(base)-len(out), rel)
		}
		return out
	}

	return base
}

func GenerateMutants(sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, log *logger.Logger) []Mutant {

	type siteKey struct {
		file string
		line int
		col  int
		ntyp uint8
	}
	seen := make(map[siteKey]bool, len(sites))

	mutants := make([]Mutant, 0, len(sites)*len(operators))
	mutantID := 1

	// Sort operators by name to ensure deterministic ordering
	sortedOps := make([]mutator.Operator, len(operators))
	copy(sortedOps, operators)
	sort.Slice(sortedOps, func(i, j int) bool {
		return sortedOps[i].Name() < sortedOps[j].Name()
	})

	for _, site := range sites {
		// 1. Apply sub-config operator override (replace semantics)
		ops := operators
		if resolver != nil && resolver.HasAnyOverrides() {
			ops = resolver.EffectiveOperators(site.File.Name(), operators, allOps)
		}

		// 2. Apply dir_rules on top (existing logic, but with merged rules)
		effectiveDirRules := dirRules
		if resolver != nil && resolver.HasAnyOverrides() {
			effectiveDirRules = resolver.EffectiveDirRules(site.File.Name(), dirRules)
		}
		ops = effectiveOperators(site.File.Name(), projectRoot, ops, effectiveDirRules, log)
		if len(ops) == 0 {
			continue
		}

		key := siteKey{
			file: site.File.Name(),
			line: site.Line,
			col:  site.Column,
			ntyp: schemata_nodes.NodeTypeToUint8(site.Node),
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		for _, op := range ops {
			if canApply(op, site) {
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

func ResolveCache(mutants []Mutant, baseDir string, c *cache.Cache) (toRun []int, fileHashes map[string]string, err error) {
	if c == nil {
		indices := make([]int, 0, len(mutants))
		for i := range mutants {
			if mutants[i].Status == "" {
				indices = append(indices, i)
			}
		}
		return indices, nil, nil
	}

	fileHashes = make(map[string]string)
	for i := range mutants {
		f := mutants[i].Site.File.Name()
		if _, ok := fileHashes[f]; !ok {
			h, err := hashFile(f)
			if err != nil {
				continue
			}
			fileHashes[f] = h
		}
	}

	var cachedCount int
	for i := range mutants {
		m := &mutants[i]
		fh := fileHashes[m.Site.File.Name()]
		if fh == "" {
			continue
		}

		key := c.Key(m.Site.File.Name(), m.Site.Line, m.Site.Column,
			schemata_nodes.NodeTypeToUint8(m.Site.Node), m.Operator.Name(), fh)
		if entry, ok := c.Get(key); ok {
			m.Status = entry.Status
			cachedCount++
		}
	}

	if cachedCount == len(mutants) {
		_ = c.Save(baseDir)
		return nil, fileHashes, nil
	}

	toRun = make([]int, 0, len(mutants)-cachedCount)
	for i := range mutants {
		if mutants[i].Status == "" {
			toRun = append(toRun, i)
		}
	}
	return toRun, fileHashes, nil
}

func SaveCache(mutants []Mutant, baseDir string, c *cache.Cache, fileHashes map[string]string) {
	if c == nil {
		return
	}

	if fileHashes == nil {
		fileHashes = make(map[string]string)
		for i := range mutants {
			f := mutants[i].Site.File.Name()
			if mutants[i].Status == "" {
				continue
			}
			if _, ok := fileHashes[f]; !ok {
				h, err := hashFile(f)
				if err != nil {
					continue
				}
				fileHashes[f] = h
			}
		}
	}

	for i := range mutants {
		m := &mutants[i]
		if m.Status == "" {
			continue
		}
		fh := fileHashes[m.Site.File.Name()]
		if fh == "" {
			continue
		}
		key := c.Key(m.Site.File.Name(), m.Site.Line, m.Site.Column,
			schemata_nodes.NodeTypeToUint8(m.Site.Node), m.Operator.Name(), fh)
		c.Set(key, m.Status)
	}
	_ = c.Save(baseDir)
}

func canApply(op mutator.Operator, site engine.Site) bool {
	if cop, ok := op.(mutator.ContextualOperator); ok {
		ctx := mutator.Context{ReturnType: site.ReturnType, EnclosingFunc: site.EnclosingFunc}
		return cop.CanApplyWithContext(site.Node, ctx)
	}
	return op.CanApply(site.Node)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	bufPtr := hashBufPool.Get().(*[]byte)
	defer hashBufPool.Put(bufPtr)

	if _, err := io.CopyBuffer(h, f, *bufPtr); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
