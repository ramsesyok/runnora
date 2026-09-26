package reporter

import (
	"encoding/xml"
	"io"
	"strings"
)

// JUnitReporter は runbook を testcase に対応付けた JUnit XML を出力する。
type JUnitReporter struct{ w io.Writer }

func NewJUnitReporter(w io.Writer) *JUnitReporter { return &JUnitReporter{w: w} }

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Detail  string `xml:",chardata"`
}

func (j *JUnitReporter) Write(r *Report) error {
	suite := junitSuite{Name: "runnora", Tests: r.Total, Failures: r.Failed}
	for _, result := range r.Results {
		entry := junitTestCase{Name: result.Path, ClassName: "runbook"}
		if !result.Passed {
			detail := result.Error
			if detail == "" {
				detail = "runbook failed"
			}
			message, _, _ := strings.Cut(detail, "\n")
			entry.Failure = &junitFailure{Message: message, Detail: detail}
		}
		suite.Cases = append(suite.Cases, entry)
	}
	data, err := xml.MarshalIndent(junitSuites{
		Tests: r.Total, Failures: r.Failed, Suites: []junitSuite{suite},
	}, "", "  ")
	if err != nil {
		return err
	}
	output := append([]byte(xml.Header), data...)
	output = append(output, '\n')
	n, err := j.w.Write(output)
	if err == nil && n != len(output) {
		return io.ErrShortWrite
	}
	return err
}

func (j *JUnitReporter) Close() error { return nil }
