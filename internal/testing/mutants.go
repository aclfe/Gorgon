package testing

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sync"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// hashBufPool provides reusable 32KB buffers for file hashing and copying.
var hashBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// GenerateMutants creates mutants from mutation sites, deduplicating by position
// and node type in a single pass, then generating one mutant per applicable operator.
func GenerateMutants(sites []engine.Site, operators []mutator.Operator) []Mutant {
	// Single-pass deduplication: O(n) instead of sort+dedup O(n log n)
	type siteKey struct {
		file string
		line int
		col  int
		ntyp uint8
	}
	seen := make(map[siteKey]bool, len(sites))
	// Pre-allocate with estimated capacity (unique sites × operators)
	mutants := make([]Mutant, 0, len(sites)*len(operators))
	mutantID := 1

	for _, site := range sites {
		key := siteKey{
			file: site.File.Name(),
			line: site.Line,
			col:  site.Column,
			ntyp: TypeToUint8(site.Node),
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		for _, op := range operators {
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

// ResolveCache checks the cache and marks already-processed mutants.
// Returns indices that still need to run, and the file hashes map (for reuse by SaveCache).
func ResolveCache(mutants []Mutant, baseDir string, c *cache.Cache) (toRun []int, fileHashes map[string]string, err error) {
	if c == nil {
		// No cache: all mutants need to run
		indices := make([]int, len(mutants))
		for i := range mutants {
			indices[i] = i
		}
		return indices, nil, nil
	}

	// Batch hash files to avoid re-reading
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
			continue // Can't hash, treat as uncached
		}

		key := c.Key(m.Site.File.Name(), m.Site.Line, m.Site.Column,
			TypeToUint8(m.Site.Node), m.Operator.Name(), fh)
		if entry, ok := c.Get(key); ok {
			m.Status = entry.Status
			cachedCount++
		}
	}

	if cachedCount == len(mutants) {
		_ = c.Save(baseDir)
		return nil, fileHashes, nil // All cached, nothing to run
	}

	// Collect indices of mutants that need to run
	toRun = make([]int, 0, len(mutants)-cachedCount)
	for i := range mutants {
		if mutants[i].Status == "" {
			toRun = append(toRun, i)
		}
	}
	return toRun, fileHashes, nil
}

// SaveCache saves mutation results. If fileHashes is nil, files are hashed.
// Pass the fileHashes map returned by ResolveCache to avoid re-hashing.
func SaveCache(mutants []Mutant, baseDir string, c *cache.Cache, fileHashes map[string]string) {
	if c == nil {
		return
	}

	// Use provided hashes or compute them
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
			TypeToUint8(m.Site.Node), m.Operator.Name(), fh)
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
