package testing

import (
    "context"
    "fmt"
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "sort"
)

const maxCompileRetries = 5

func compileWithSurgicalRetry(
	ctx context.Context,
	executor *testExecutor,
	mutantIDs []int,
	initialSites map[int]MutantSite, 
	pkgMutants []*Mutant,
	projectRoot string,
) compileResultWithAttribution {

	finalResult := compileResultWithAttribution{
		perMutant: make(map[int]error, len(mutantIDs)),
	}

	currentIDs := make([]int, len(mutantIDs))
	copy(currentIDs, mutantIDs)

	mutantByID := make(map[int]*Mutant, len(pkgMutants))
	for _, m := range pkgMutants {
		mutantByID[m.ID] = m
	}

	for attempt := 0; attempt < maxCompileRetries && len(currentIDs) > 0; attempt++ {

		
		currentSites := rebuildMutantSites(pkgMutants)

		result := executor.compileWithAttribution(ctx, currentIDs, currentSites)

		var errorIDs, cleanIDs []int
		for _, id := range currentIDs {
			if result.perMutant[id] != nil {
				errorIDs = append(errorIDs, id)
				finalResult.perMutant[id] = result.perMutant[id]
			} else {
				cleanIDs = append(cleanIDs, id)
			}
		}

		if result.compileFailed && len(errorIDs) == 0 {
			guilty := isolateByBinarySearch(ctx, executor, cleanIDs, currentSites, pkgMutants, projectRoot)

			guiltySet := make(map[int]struct{}, len(guilty))
			for _, id := range guilty {
				guiltySet[id] = struct{}{}
				finalResult.perMutant[id] = fmt.Errorf("compilation failed (isolated via binary search)")
				if m, ok := mutantByID[id]; ok {
					m.Status = "error"
				}
			}

			nonGuiltyIDs := make([]int, 0, len(cleanIDs))
			for _, id := range cleanIDs {
				if _, isGuilty := guiltySet[id]; !isGuilty {
					nonGuiltyIDs = append(nonGuiltyIDs, id)
					finalResult.perMutant[id] = nil
				}
			}

			if len(nonGuiltyIDs) > 0 {
				nonGuiltyMutants := mutantsByIDs(pkgMutants, nonGuiltyIDs)
				if err := reApplySchemataToPkg(nonGuiltyMutants, projectRoot, executor.tempDir); err == nil {
					finalSites := rebuildMutantSites(pkgMutants)
					executor.compileWithAttribution(ctx, nonGuiltyIDs, finalSites)
				} else {
					for _, id := range nonGuiltyIDs {
						finalResult.perMutant[id] = fmt.Errorf("schemata rebuild after isolation failed: %w", err)
					}
				}
			}

			return finalResult
		}

		if len(errorIDs) == 0 {
			for _, id := range cleanIDs {
				finalResult.perMutant[id] = nil
			}
			return finalResult
		}

		if len(errorIDs) > len(currentIDs)*3/5 {
			for _, id := range cleanIDs {
				finalResult.perMutant[id] = nil
			}
			for _, id := range errorIDs {
				if m, ok := mutantByID[id]; ok {
					m.Status = "error"
				}
			}
			return finalResult
		}

		if len(cleanIDs) == 0 {
			return finalResult
		}

		cleanMutants := mutantsByIDs(pkgMutants, cleanIDs)
		if err := reApplySchemataToPkg(cleanMutants, projectRoot, executor.tempDir); err != nil {
			for _, id := range cleanIDs {
				finalResult.perMutant[id] = fmt.Errorf("schemata rebuild failed after error isolation: %w", err)
			}
			return finalResult
		}

		currentIDs = cleanIDs
	}

	
	for _, id := range currentIDs {
		if _, exists := finalResult.perMutant[id]; !exists {
			finalResult.perMutant[id] = fmt.Errorf("max compile retries (%d) exceeded isolating bad mutations", maxCompileRetries)
		}
	}
	return finalResult
}

func reApplySchemataToPkg(cleanMutants []*Mutant, projectRoot, tempDir string) error {
	astToMutants := make(map[*ast.File][]*Mutant)
	for _, m := range cleanMutants {
		if m.Site.FileAST != nil {
			astToMutants[m.Site.FileAST] = append(astToMutants[m.Site.FileAST], m)
		}
	}

	type entry struct {
		astFile *ast.File
		mutants []*Mutant
	}
	entries := make([]entry, 0, len(astToMutants))
	for af, ms := range astToMutants {
		entries = append(entries, entry{af, ms})
	}
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].mutants) == 0 || len(entries[j].mutants) == 0 {
			return false
		}
		return entries[i].mutants[0].Site.File.Name() < entries[j].mutants[0].Site.File.Name()
	})

	for _, e := range entries {
		if len(e.mutants) == 0 || e.mutants[0].Site.File == nil {
			continue
		}
		origPath := e.mutants[0].Site.File.Name()
		src, err := os.ReadFile(origPath)
		if err != nil {
			return fmt.Errorf("re-read %s: %w", origPath, err)
		}
		rel, err := filepath.Rel(projectRoot, origPath)
		if err != nil {
			return fmt.Errorf("rel path %s: %w", origPath, err)
		}
		tempFile := filepath.Join(tempDir, rel)

		
		
		
		
		freshFset := token.NewFileSet()
		freshAST, parseErr := parser.ParseFile(freshFset, origPath, src, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("re-parse %s: %w", origPath, parseErr)
		}

		posMap, err := ApplySchemataToAST(freshAST, freshFset, tempFile, src, e.mutants)
		if err != nil {
			return fmt.Errorf("ApplySchemataToAST on %s: %w", tempFile, err)
		}
		for _, m := range e.mutants {
			if pm, ok := posMap[m.ID]; ok {
				m.TempLine = pm.TempLine
				m.TempCol = pm.TempCol
			}
		}
	}

	fileToMutants := make(map[string][]*Mutant)
	for _, m := range cleanMutants {
		if m.Site.File == nil {
			continue
		}
		rel, err := filepath.Rel(projectRoot, m.Site.File.Name())
		if err != nil {
			continue
		}
		tempFile := filepath.Join(tempDir, rel)
		fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
	}
	return InjectSchemataHelpers(fileToMutants)
}

func mutantsByIDs(mutants []*Mutant, ids []int) []*Mutant {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	out := make([]*Mutant, 0, len(ids))
	for _, m := range mutants {
		if _, ok := set[m.ID]; ok {
			out = append(out, m)
		}
	}
	return out
}




func isolateByBinarySearch(
	ctx context.Context,
	executor *testExecutor,
	mutantIDs []int,
	_ map[int]MutantSite, 
	pkgMutants []*Mutant,
	projectRoot string,
) []int {
	if len(mutantIDs) == 0 {
		return nil
	}
	if len(mutantIDs) == 1 {
		return mutantIDs
	}
	if len(mutantIDs) == 2 {
		var guilty []int
		for _, id := range mutantIDs {
			currentSites := rebuildMutantSites(pkgMutants)
			result := executor.compileWithAttribution(ctx, []int{id}, currentSites)
			if result.compileFailed || result.perMutant[id] != nil {
				guilty = append(guilty, id)
			}
		}
		return guilty
	}

	mid := len(mutantIDs) / 2
	firstHalf := mutantIDs[:mid]
	secondHalf := mutantIDs[mid:]

	var guilty []int

	
	firstMutants := mutantsByIDs(pkgMutants, firstHalf)
	if len(firstMutants) > 0 {
		if err := reApplySchemataToPkg(firstMutants, projectRoot, executor.tempDir); err == nil {
			currentSites := rebuildMutantSites(pkgMutants)
			result := executor.compileWithAttribution(ctx, firstHalf, currentSites)
			if result.compileFailed {
				guilty = append(guilty, isolateByBinarySearch(ctx, executor, firstHalf, nil, pkgMutants, projectRoot)...)
			} else {
				for _, id := range firstHalf {
					if result.perMutant[id] != nil {
						guilty = append(guilty, id)
					}
				}
			}
		}
	}

	
	secondMutants := mutantsByIDs(pkgMutants, secondHalf)
	if len(secondMutants) > 0 {
		if err := reApplySchemataToPkg(secondMutants, projectRoot, executor.tempDir); err == nil {
			currentSites := rebuildMutantSites(pkgMutants)
			result := executor.compileWithAttribution(ctx, secondHalf, currentSites)
			if result.compileFailed {
				guilty = append(guilty, isolateByBinarySearch(ctx, executor, secondHalf, nil, pkgMutants, projectRoot)...)
			} else {
				for _, id := range secondHalf {
					if result.perMutant[id] != nil {
						guilty = append(guilty, id)
					}
				}
			}
		}
	}

	return guilty
}

func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func rebuildMutantSites(pkgMutants []*Mutant) map[int]MutantSite {
	sites := make(map[int]MutantSite, len(pkgMutants))
	for _, m := range pkgMutants {
		if m.Site.File == nil {
			continue
		}
		line := m.TempLine
		if line == 0 {
			line = m.Site.Line
		}
		col := m.TempCol
		if col == 0 {
			col = m.Site.Column
		}
		sites[m.ID] = MutantSite{
			File: m.Site.File.Name(),
			Line: line,
			Col:  col,
		}
	}
	return sites
}