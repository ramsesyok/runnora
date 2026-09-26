package reporter_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/reporter"
)

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestReport_HasRunResults(t *testing.T) {
	r := &reporter.Report{
		Total:  3,
		Passed: 2,
		Failed: 1,
		Results: []reporter.RunResult{
			{Path: "a.yml", Passed: true},
			{Path: "b.yml", Passed: true},
			{Path: "c.yml", Passed: false, Error: "assertion failed"},
		},
	}
	if r.Total != 3 {
		t.Errorf("got Total=%d, want 3", r.Total)
	}
	if len(r.Results) != 3 {
		t.Errorf("got %d results, want 3", len(r.Results))
	}
}

func TestJSONReporter_WritesResults(t *testing.T) {
	var buf bytes.Buffer
	rep := &reporter.Report{
		Total: 2, Passed: 1, Failed: 1,
		Results: []reporter.RunResult{
			{Path: "ok.yml", Passed: true},
			{Path: "failed.yml", Passed: false, Error: "status was 500"},
		},
	}
	if err := reporter.NewJSONReporter(&buf).Write(rep); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Results []struct {
			Path   string `json:"path"`
			Passed bool   `json:"passed"`
			Error  string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, buf.String())
	}
	if got.Total != 2 || got.Passed != 1 || got.Failed != 1 || len(got.Results) != 2 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if got.Results[0].Path != "ok.yml" || !got.Results[0].Passed || got.Results[1].Error != "status was 500" {
		t.Fatalf("unexpected results: %+v", got.Results)
	}
}

func TestJUnitReporter_WritesEscapedFailure(t *testing.T) {
	var buf bytes.Buffer
	detail := "expected <book> & actual\nsecond line"
	rep := &reporter.Report{
		Total: 2, Passed: 1, Failed: 1,
		Results: []reporter.RunResult{
			{Path: "ok.yml", Passed: true},
			{Path: "book<&>.yml", Passed: false, Error: detail},
		},
	}
	if err := reporter.NewJUnitReporter(&buf).Write(rep); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte(xml.Header)) {
		t.Fatalf("missing XML declaration: %q", buf.String())
	}
	var got struct {
		XMLName  xml.Name `xml:"testsuites"`
		Tests    int      `xml:"tests,attr"`
		Failures int      `xml:"failures,attr"`
		Suite    struct {
			Name  string `xml:"name,attr"`
			Cases []struct {
				Name    string `xml:"name,attr"`
				Failure *struct {
					Message string `xml:"message,attr"`
					Detail  string `xml:",chardata"`
				} `xml:"failure"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JUnit XML: %v\n%s", err, buf.String())
	}
	if got.Tests != 2 || got.Failures != 1 || got.Suite.Name != "runnora" || len(got.Suite.Cases) != 2 {
		t.Fatalf("unexpected suite: %+v", got)
	}
	if got.Suite.Cases[0].Failure != nil || got.Suite.Cases[1].Name != "book<&>.yml" {
		t.Fatalf("unexpected test cases: %+v", got.Suite.Cases)
	}
	if got.Suite.Cases[1].Failure == nil || got.Suite.Cases[1].Failure.Message != "expected <book> & actual" || got.Suite.Cases[1].Failure.Detail != detail {
		t.Fatalf("unexpected failure: %+v", got.Suite.Cases[1].Failure)
	}
}

func TestNewFileReporter_SelectsFormatAndRejectsUnknown(t *testing.T) {
	rep := &reporter.Report{Total: 1, Passed: 1, Results: []reporter.RunResult{{Path: "ok.yml", Passed: true}}}
	for _, tc := range []struct {
		format string
		prefix string
	}{
		{format: "json", prefix: "{\n"},
		{format: "junit", prefix: xml.Header},
	} {
		t.Run(tc.format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report")
			r, err := reporter.NewFileReporter(tc.format, path)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Write(rep); err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(content), tc.prefix) {
				t.Fatalf("unexpected %s report: %q", tc.format, content)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "invalid")
	if _, err := reporter.NewFileReporter("yaml", path); err == nil {
		t.Fatal("unsupported format should fail")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unsupported format created an output file: %v", err)
	}
}

func TestTextReporter_WritesSummaryLine(t *testing.T) {
	var buf bytes.Buffer
	tr := reporter.NewTextReporter(&buf)

	rep := &reporter.Report{Total: 3, Passed: 2, Failed: 1}
	if err := tr.Write(rep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "3") {
		t.Errorf("output missing Total count: %q", out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("output missing Passed count: %q", out)
	}
	if !strings.Contains(out, "1") {
		t.Errorf("output missing Failed count: %q", out)
	}
}

func TestTextReporter_PropagatesWriteError(t *testing.T) {
	want := errors.New("write failed")
	if err := reporter.NewTextReporter(errorWriter{err: want}).Write(&reporter.Report{}); !errors.Is(err, want) {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestTextReporter_WritesFailureDetail(t *testing.T) {
	var buf bytes.Buffer
	tr := reporter.NewTextReporter(&buf)

	rep := &reporter.Report{
		Total:  1,
		Failed: 1,
		Results: []reporter.RunResult{
			{Path: "user_create.yml", Passed: false, Error: "assertion failed at step 3"},
		},
	}
	if err := tr.Write(rep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "user_create.yml") {
		t.Errorf("output missing runbook path: %q", out)
	}
	if !strings.Contains(out, "assertion failed") {
		t.Errorf("output missing error detail: %q", out)
	}
}

func TestTextReporter_AllPassedNoFailureDetail(t *testing.T) {
	var buf bytes.Buffer
	tr := reporter.NewTextReporter(&buf)

	rep := &reporter.Report{
		Total:  2,
		Passed: 2,
		Results: []reporter.RunResult{
			{Path: "a.yml", Passed: true},
			{Path: "b.yml", Passed: true},
		},
	}
	if err := tr.Write(rep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "FAIL") {
		t.Errorf("output should not contain FAIL when all pass: %q", out)
	}
}

func TestNewFileReporter_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.txt")

	r, err := reporter.NewFileReporter("text", outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	rep := &reporter.Report{Total: 1, Passed: 1, Results: []reporter.RunResult{{Path: "a.yml", Passed: true}}}
	if err := r.Write(rep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r.Close()

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), "1") {
		t.Errorf("file content wrong: %q", string(content))
	}
}
