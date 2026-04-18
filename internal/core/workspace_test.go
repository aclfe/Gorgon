package testing

import (
	"go/token"
	"testing"
)

func TestChunkingLargeFiles(t *testing.T) {
	fset := token.NewFileSet()
	file := fset.AddFile("test.go", -1, 1000)
	
	mutants := make([]Mutant, 550)
	for i := range mutants {
		mutants[i] = Mutant{
			ID: i + 1,
		}
		mutants[i].Site.File = file
	}

	t.Run("chunking enabled", func(t *testing.T) {
		const maxMutantsPerFile = 500
		chunkLargeFiles := true

		type astEntry struct {
			mutants    []*Mutant
			chunkIndex int
		}

		var entries []astEntry
		mutantPtrs := make([]*Mutant, len(mutants))
		for i := range mutants {
			mutantPtrs[i] = &mutants[i]
		}

		if !chunkLargeFiles || len(mutantPtrs) <= maxMutantsPerFile {
			entries = append(entries, astEntry{mutantPtrs, 0})
		} else {
			chunkIdx := 1
			for i := 0; i < len(mutantPtrs); i += maxMutantsPerFile {
				end := i + maxMutantsPerFile
				if end > len(mutantPtrs) {
					end = len(mutantPtrs)
				}
				entries = append(entries, astEntry{mutantPtrs[i:end], chunkIdx})
				chunkIdx++
			}
		}

		if len(entries) != 2 {
			t.Errorf("Expected 2 chunks, got %d", len(entries))
		}
		if len(entries[0].mutants) != 500 {
			t.Errorf("Expected first chunk to have 500 mutants, got %d", len(entries[0].mutants))
		}
		if len(entries[1].mutants) != 50 {
			t.Errorf("Expected second chunk to have 50 mutants, got %d", len(entries[1].mutants))
		}
		if entries[0].chunkIndex != 1 {
			t.Errorf("Expected first chunk index to be 1, got %d", entries[0].chunkIndex)
		}
		if entries[1].chunkIndex != 2 {
			t.Errorf("Expected second chunk index to be 2, got %d", entries[1].chunkIndex)
		}
	})

	t.Run("chunking disabled", func(t *testing.T) {
		const maxMutantsPerFile = 500
		chunkLargeFiles := false

		type astEntry struct {
			mutants    []*Mutant
			chunkIndex int
		}

		var entries []astEntry
		mutantPtrs := make([]*Mutant, len(mutants))
		for i := range mutants {
			mutantPtrs[i] = &mutants[i]
		}

		if !chunkLargeFiles || len(mutantPtrs) <= maxMutantsPerFile {
			entries = append(entries, astEntry{mutantPtrs, 0})
		} else {
			chunkIdx := 1
			for i := 0; i < len(mutantPtrs); i += maxMutantsPerFile {
				end := i + maxMutantsPerFile
				if end > len(mutantPtrs) {
					end = len(mutantPtrs)
				}
				entries = append(entries, astEntry{mutantPtrs[i:end], chunkIdx})
				chunkIdx++
			}
		}

		if len(entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(entries))
		}
		if len(entries[0].mutants) != 550 {
			t.Errorf("Expected entry to have 550 mutants, got %d", len(entries[0].mutants))
		}
		if entries[0].chunkIndex != 0 {
			t.Errorf("Expected chunk index to be 0, got %d", entries[0].chunkIndex)
		}
	})
}

