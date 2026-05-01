package reporter

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	testing "github.com/aclfe/gorgon/internal/core"
)

type junitTestSuite struct {
	XMLName xml.Name `xml:"testsuite"`
	Name    string   `xml:"name,attr"`
	ReportStats
	Time      float64         `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	XMLName   xml.Name `xml:"testcase"`
	Name      string   `xml:"name,attr"`
	Classname string   `xml:"classname,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	XMLName xml.Name `xml:"failure"`
	Message string   `xml:"message,attr"`
	Text    string   `xml:",chardata"`
}

type junitSkipped struct {
	XMLName xml.Name `xml:"skipped"`
	Message string   `xml:"message,attr"`
}

func writeJUnitReport(mutants []testing.Mutant, stats ReportStats, outputFile string) error {
	suite := junitTestSuite{
		Name:        "Mutation Testing",
		ReportStats: stats,
	}

	for _, m := range mutants {
		tc := junitTestCase{
			Name:      fmt.Sprintf("%s:%d:%d", filepath.Base(m.Site.File.Name()), m.Site.Line, m.Site.Column),
			Classname: m.Operator.Name(),
		}

		switch m.Status {
		case testing.StatusKilled:
			tc.Time = m.KillDuration.Seconds()
		case testing.StatusSurvived:
			tc.Failure = &junitFailure{
				Message: "Mutant survived",
				Text:    formatMutantInfo(m),
			}
		case testing.StatusTimeout:
			tc.Failure = &junitFailure{
				Message: "Mutant timeout",
				Text:    formatMutantInfo(m),
			}
		case testing.StatusError:
			errMsg := ""
			if m.Error != nil {
				errMsg = m.Error.Error()
			}
			tc.Failure = &junitFailure{
				Message: "Execution error",
				Text:    errMsg,
			}
		case testing.StatusUntested:
			tc.Skipped = &junitSkipped{
				Message: "Package failed to compile or has no tests",
			}
		case testing.StatusInvalid:
			tc.Skipped = &junitSkipped{
				Message: "Mutant marked invalid",
			}
		}

		suite.TestCases = append(suite.TestCases, tc)
	}

	data, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputFile, append([]byte(xml.Header), data...), 0644)
}

func formatMutantInfo(m testing.Mutant) string {
	return fmt.Sprintf("Operator: %s\nFile: %s:%d\nMutant ID: %d", m.Operator.Name(), m.Site.File.Name(), m.Site.Line, m.ID)
}
