package reporter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/reporter"
)

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
