package testing

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/mutator"
)



func GenerateMutants(sites []engine.Site, operators []mutator.Operator) []Mutant {

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

		for _, op := range sortedOps {
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
		
		indices := make([]int, len(mutants))
		for i := range mutants {
			indices[i] = i
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
