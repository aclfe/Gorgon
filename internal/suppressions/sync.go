

package suppressions

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/config"
)



func SyncSuppressions(configPath string, eng *engine.Engine) {
	directives := eng.IgnoreDirectives()
	if len(directives) == 0 {
		return
	}
	if configPath == "" {
		return
	}

	_, err := os.Stat(configPath)
	fileExists := err == nil

	var cfg *config.Config
	if fileExists {
		cfg, err = config.Load(configPath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to load config for suppress sync: %v\n", err)
			return
		}
	} else {
		cfg = config.Default()
	}

	
	existingConfigSuppress := buildSuppressMap(cfg.Suppress)

	
	projectRoot := eng.ProjectRoot()
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}

	mergeInlineDirectives(existingConfigSuppress, directives, projectRoot)

	
	cfg.Suppress = buildSuppressEntries(existingConfigSuppress)

	if err := cfg.Save(configPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save config: %v\n", err)
	}
}

func buildSuppressMap(entries []config.SuppressEntry) map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	for _, entry := range entries {
		if result[entry.Location] == nil {
			result[entry.Location] = make(map[string]bool)
		}
		for _, op := range entry.Operators {
			result[entry.Location][op] = true
		}
	}
	return result
}

func mergeInlineDirectives(
	existing map[string]map[string]bool,
	directives map[string]map[int]map[string]map[int]bool,
	projectRoot string,
) {
	for absPath, lineMap := range directives {
		relPath := absPath
		if r, err := filepath.Rel(projectRoot, absPath); err == nil {
			relPath = r
		}

		for line, opMap := range lineMap {
			location := fmt.Sprintf("%s:%d", relPath, line)

			if existing[location] == nil {
				existing[location] = make(map[string]bool)
			}

			for op, colMap := range opMap {
				if op == "" {
					
					existing[location][""] = true
					continue
				}
				for col := range colMap {
					if col == 0 {
						existing[location][op] = true
					} else {
						existing[location][fmt.Sprintf("%s:%d", op, col)] = true
					}
				}
			}
		}
	}
}

func buildSuppressEntries(suppressMap map[string]map[string]bool) []config.SuppressEntry {
	var entries []config.SuppressEntry
	for location, ops := range suppressMap {
		hasAllOps := ops[""]
		delete(ops, "")

		var operators []string
		for op := range ops {
			operators = append(operators, op)
		}

		if hasAllOps {
			operators = nil
		}

		entries = append(entries, config.SuppressEntry{
			Location:  location,
			Operators: operators,
		})
	}
	return entries
}
