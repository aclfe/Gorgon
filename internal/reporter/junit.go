package reporter

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aclfe/gorgon/internal/core"
)

type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
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

func writeJUnitReport(mutants []testing.Mutant, outputFile string) error {
	suite := junitTestSuite{
		Name:  "Mutation Testing",
		Tests: len(mutants),
	}

	for _, m := range mutants {
		tc := junitTestCase{
			Name:      fmt.Sprintf("%s:%d:%d", filepath.Base(m.Site.File.Name()), m.Site.Line, m.Site.Column),
			Classname: m.Operator.Name(),
		}

		switch m.Status {
		case "killed":
			tc.Time = m.KillDuration.Seconds()
		case "survived":
			suite.Failures++
			tc.Failure = &junitFailure{
				Message: "Mutant survived",
				Text:    fmt.Sprintf("Operator: %s\nFile: %s:%d\nMutant ID: %d", m.Operator.Name(), m.Site.File.Name(), m.Site.Line, m.ID),
			}
		case "error":
			suite.Errors++
			errMsg := ""
			if m.Error != nil {
				errMsg = m.Error.Error()
			}
			tc.Failure = &junitFailure{
				Message: "Compilation error",
				Text:    errMsg,
			}
		case "untested":
			suite.Skipped++
			tc.Skipped = &junitSkipped{
				Message: "Binary missing - package failed to compile",
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
