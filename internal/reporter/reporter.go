package reporter

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/aclfe/gorgon/internal/testing"
)

const (
	percentageMultiplier = 100
	tabWidth             = 4
)

func Report(mutants []testing.Mutant, threshold float64, debug bool) error {
	total := len(mutants)
	killed := 0
	survived := 0
	errors := 0

	for _, mutant := range mutants {
		switch mutant.Status {
		case "killed":
			killed++
		case "survived":
			survived++
		case "error":
			errors++
		default:
			
		}
	}

	
	fileCache := make(map[string][]byte)

	
	if debug {
		fmt.Println("=== Debug Information ===")

		
		if errors > 0 {
			fmt.Printf("\nError Mutants (%d total):\n", errors)
			
			type locKey struct {
				file string
				line int
				col  int
			}
			type locGroup struct {
				mutants  []testing.Mutant
				errMsg   string
			}
			groups := make(map[locKey]*locGroup)
			var order []locKey
			for _, mutant := range mutants {
				if mutant.Status == "error" && mutant.Error != nil {
					key := locKey{
						file: mutant.Site.File.Name(),
						line: mutant.Site.Line,
						col:  mutant.Site.Column,
					}
					if _, ok := groups[key]; !ok {
						groups[key] = &locGroup{errMsg: mutant.Error.Error()}
						order = append(order, key)
					}
					groups[key].mutants = append(groups[key].mutants, mutant)
				}
			}

			for _, key := range order {
				g := groups[key]
				baseName := filepath.Base(key.file)
				fmt.Printf("  %s:%d:%d  ", baseName, key.line, key.col)
				for i, m := range g.mutants {
					if i > 0 {
						fmt.Print(", ")
					}
					fmt.Printf("%s (#%d)", m.Operator.Name(), m.ID)
				}
				fmt.Println()
			}

			
			uniqueErrors := extractUniqueCompilerErrors(mutants)
			if len(uniqueErrors) > 0 {
				fmt.Printf("\nCompilation Error Context:\n")
				for _, errMsg := range uniqueErrors {
					fmt.Printf("  %s\n", errMsg)
				}
			}
		}

		fmt.Println("\nSurvived Mutants:")
		if survived == 0 {
			fmt.Println("  (none)")
		} else {
			for _, mutant := range mutants {
				if mutant.Status == "survived" {
					col := getVisualColumn(fileCache, mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column)
					baseName := filepath.Base(mutant.Site.File.Name())
					fmt.Printf("  %s:%d:%d (%s)\n",
						baseName,
						mutant.Site.Line,
						col,
						mutant.Operator.Name())
				}
			}
		}

		fmt.Println("\n=== End Debug Information ===")
		fmt.Println("---")
	}

	score := float64(killed) / float64(total) * percentageMultiplier

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "Mutation Score\tKilled\tSurvived\tErrors\tTotal"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "%.2f%%\t%d\t%d\t%d\t%d\n", score, killed, survived, errors, total); err != nil {
		return fmt.Errorf("failed to write stats: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	
	if !debug {
		fmt.Println("\nSurvived Mutants:")
		for _, mutant := range mutants {
			if mutant.Status == "survived" {
				col := getVisualColumn(fileCache, mutant.Site.File.Name(), mutant.Site.Line, mutant.Site.Column)
				fmt.Printf("- %s in %s:%d:%d (Operator: %s)\n",
					mutant.Status,
					mutant.Site.File.Name(),
					mutant.Site.Line,
					col,
					mutant.Operator.Name())
			}
		}
	}

	if threshold > 0 && score < threshold {
		return fmt.Errorf("mutation score %.2f%% is below threshold %.2f%%", score, threshold)
	}

	
	
	return nil
}

func extractUniqueCompilerErrors(mutants []testing.Mutant) []string {
	seen := make(map[string]bool)
	var unique []string
	prefix := "compilation failed: "
	for _, m := range mutants {
		if m.Status == "error" && m.Error != nil {
			msg := m.Error.Error()
			if idx := len(prefix); len(msg) > idx && msg[:idx] == prefix {
				msg = msg[idx:]
			}
			for _, errMsg := range testing.UniqueErrorLines(msg, "# ") {
				if !seen[errMsg] {
					seen[errMsg] = true
					unique = append(unique, errMsg)
				}
			}
		}
	}
	return unique
}

func calculateVisualColumn(content []byte, line, col int) int {
	start := 0
	currentLine := 1
	for i, b := range content {
		if currentLine == line {
			start = i
			break
		}
		if b == '\n' {
			currentLine++
		}
	}

	visualCol := 1
	for i := 0; i < col-1; i++ {
		if start+i >= len(content) {
			break
		}
		if content[start+i] == '\t' {
			visualCol += tabWidth - (visualCol-1)%tabWidth
		} else {
			visualCol++
		}
	}
	return visualCol
}

func getVisualColumn(fileCache map[string][]byte, fileName string, line, col int) int {
	if content, ok := fileCache[fileName]; ok {
		return calculateVisualColumn(content, line, col)
	}
	if content, err := os.ReadFile(fileName); err == nil {
		fileCache[fileName] = content
		return calculateVisualColumn(content, line, col)
	}
	return col
}
