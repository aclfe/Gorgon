// Package reporter provides functionality to report mutation testing results.
package reporter

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aclfe/gorgon/internal/testing"
)

const (
	percentageMultiplier = 100
	tabWidth             = 4
)

func Report(mutants []testing.Mutant) error {
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
			// Should not happen, but good for completeness
		}
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

	fmt.Println("\nSurvived Mutants:")
	for _, mutant := range mutants {
		if mutant.Status == "survived" {
			pos := mutant.Site.File.Position(mutant.Site.Pos)
			col := pos.Column
			if content, err := os.ReadFile(mutant.Site.File.Name()); err == nil {
				col = calculateVisualColumn(content, pos.Line, pos.Column)
			}
			fmt.Printf("- %s in %s:%d:%d (Operator: %s)\n",
				mutant.Status,
				mutant.Site.File.Name(),
				pos.Line,
				col,
				mutant.Operator.Name())
		}
	}

	// WILL MAKE: HTML output with flag
	// we'll have to make it pretty too kinda
	return nil
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
