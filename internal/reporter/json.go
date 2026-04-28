package reporter

import (
	"encoding/json"
	"os"

	"github.com/aclfe/gorgon/internal/core"
)

type jsonReport struct {
	Summary ReportStats  `json:"summary"`
	Mutants []jsonMutant `json:"mutants"`
}

type jsonMutant struct {
	ID       int    `json:"id"`
	Status   string `json:"status"`
	Operator string `json:"operator"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	KilledBy string `json:"killed_by,omitempty"`
	Error    string `json:"error,omitempty"`
}

func writeJSONReport(mutants []testing.Mutant, stats ReportStats, outputFile string) error {
	report := jsonReport{
		Summary: stats,
		Mutants: make([]jsonMutant, 0, len(mutants)),
	}

	for _, m := range mutants {
		jm := jsonMutant{
			ID:       m.ID,
			Status:   m.Status,
			Operator: m.Operator.Name(),
			File:     m.Site.File.Name(),
			Line:     m.Site.Line,
			Column:   m.Site.Column,
		}
		if m.KilledBy != "" {
			jm.KilledBy = m.KilledBy
		}
		if m.Error != nil {
			jm.Error = m.Error.Error()
		}
		report.Mutants = append(report.Mutants, jm)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputFile, data, 0644)
}
