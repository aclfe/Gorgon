package reporter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aclfe/gorgon/internal/core"
)

type sarifResult struct {
	RuleID    string      `json:"ruleId"`
	Message   sarifMessage `json:"message"`
	Locations []sarifLocation `json:"locations"`
	Level     string      `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

type sarifRule struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ShortDescription sarifText `json:"shortDescription"`
	HelpURI         string `json:"helpUri,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRun struct {
	Tool    sarifTool    `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name  string      `json:"name"`
	Rules []sarifRule `json:"rules"`
}

type sarifLog struct {
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

func writeSARIFReport(mutants []testing.Mutant, outputFile string) error {
	ruleMap := make(map[string]bool)
	var results []sarifResult
	var rules []sarifRule

	for _, m := range mutants {
		if m.Status == "survived" {
			ruleID := m.Operator.Name()
			
			// Add rule if not seen
			if !ruleMap[ruleID] {
				ruleMap[ruleID] = true
				rules = append(rules, sarifRule{
					ID:   ruleID,
					Name: ruleID,
					ShortDescription: sarifText{
						Text: fmt.Sprintf("Mutation operator: %s", ruleID),
					},
				})
			}

			// Add result for survived mutant
			results = append(results, sarifResult{
				RuleID: ruleID,
				Message: sarifMessage{
					Text: fmt.Sprintf("Mutant survived: %s at line %d", ruleID, m.Site.Line),
				},
				Locations: []sarifLocation{
					{
						PhysicalLocation: sarifPhysicalLocation{
							ArtifactLocation: sarifArtifactLocation{
								URI: m.Site.File.Name(),
							},
							Region: sarifRegion{
								StartLine:   m.Site.Line,
								StartColumn: m.Site.Column,
							},
						},
					},
				},
				Level: "warning",
			})
		}
	}

	log := sarifLog{
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:  "Gorgon",
						Rules: rules,
					},
				},
				Results: results,
			},
		},
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputFile, data, 0644)
}
