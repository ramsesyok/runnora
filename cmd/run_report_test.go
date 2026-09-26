package cmd_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
	"github.com/ramsesyok/runnora/internal/app"
)

func writeReportRunbook(t *testing.T, assertion string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	bookPath := filepath.Join(dir, "book.yml")
	if err := os.WriteFile(configPath, []byte("app: {name: report-test}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bookPath, []byte("steps:\n  check:\n    test: "+assertion+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, bookPath
}

func TestRunCmd_ReportFormatOnStdoutAndFile(t *testing.T) {
	configPath, bookPath := writeReportRunbook(t, "true")
	for _, format := range []string{"json", "junit"} {
		for _, toFile := range []bool{false, true} {
			name := format + "/stdout"
			if toFile {
				name = format + "/file"
			}
			t.Run(name, func(t *testing.T) {
				root := cmd.NewRootCmd()
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				args := []string{"run", "--config", configPath, "--report-format", format}
				outPath := filepath.Join(t.TempDir(), "report")
				if toFile {
					args = append(args, "--report-out", outPath)
				}
				root.SetArgs(append(args, bookPath))
				if err := root.Execute(); err != nil {
					t.Fatalf("run failed: %v\n%s", err, stderr.String())
				}
				content := stdout.Bytes()
				if toFile {
					var err error
					content, err = os.ReadFile(outPath)
					if err != nil {
						t.Fatal(err)
					}
					if stdout.Len() != 0 {
						t.Fatalf("report also written to stdout: %q", stdout.String())
					}
				}
				if format == "json" {
					var got struct {
						Total   int `json:"total"`
						Passed  int `json:"passed"`
						Failed  int `json:"failed"`
						Results []struct {
							Path   string `json:"path"`
							Passed bool   `json:"passed"`
						} `json:"results"`
					}
					if err := json.Unmarshal(content, &got); err != nil {
						t.Fatalf("unexpected JSON report: %v %q", err, content)
					}
					if got.Total != 1 || got.Passed != 1 || got.Failed != 0 || len(got.Results) != 1 || got.Results[0].Path != bookPath || !got.Results[0].Passed {
						t.Fatalf("incorrect JSON results: %+v", got)
					}
				} else {
					var got struct {
						XMLName  xml.Name `xml:"testsuites"`
						Tests    int      `xml:"tests,attr"`
						Failures int      `xml:"failures,attr"`
						Suite    struct {
							Cases []struct {
								Name string `xml:"name,attr"`
							} `xml:"testcase"`
						} `xml:"testsuite"`
					}
					if err := xml.Unmarshal(content, &got); err != nil {
						t.Fatalf("unexpected JUnit report: %v %q", err, content)
					}
					if got.Tests != 1 || got.Failures != 0 || len(got.Suite.Cases) != 1 || got.Suite.Cases[0].Name != bookPath {
						t.Fatalf("incorrect JUnit results: %+v", got)
					}
				}
			})
		}
	}
}

func TestRunCmd_FailureStillWritesJUnitReport(t *testing.T) {
	configPath, bookPath := writeReportRunbook(t, "false")
	outPath := filepath.Join(t.TempDir(), "failed.xml")
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"run", "--config", configPath, "--report-format", "junit", "--report-out", outPath, bookPath})
	root.SetErr(&bytes.Buffer{})
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	var appErr *app.AppError
	if err := root.Execute(); !errors.As(err, &appErr) || appErr.ExitCode != 1 {
		t.Fatalf("expected runbook failure (exit 1), got %v", err)
	}
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Failures int `xml:"failures,attr"`
		Suite    struct {
			Cases []struct {
				Name    string `xml:"name,attr"`
				Failure *struct {
					Message string `xml:"message,attr"`
					Detail  string `xml:",chardata"`
				} `xml:"failure"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal(content, &got); err != nil {
		t.Fatalf("invalid JUnit report: %v %q", err, content)
	}
	if got.Failures != 1 || len(got.Suite.Cases) != 1 || got.Suite.Cases[0].Name != bookPath || got.Suite.Cases[0].Failure == nil || got.Suite.Cases[0].Failure.Message == "" || got.Suite.Cases[0].Failure.Detail == "" {
		t.Fatalf("failed runbook missing from JUnit report: %+v", got)
	}
}

func TestRunCmd_UnsupportedFormatFailsBeforeExecution(t *testing.T) {
	configPath, bookPath := writeReportRunbook(t, "true")
	outPath := filepath.Join(t.TempDir(), "invalid.xml")
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"run", "--config", configPath, "--report-format", "yaml", "--report-out", outPath, bookPath})
	root.SetErr(&bytes.Buffer{})
	var appErr *app.AppError
	if err := root.Execute(); !errors.As(err, &appErr) || appErr.ExitCode != 2 || !strings.Contains(err.Error(), "unsupported report format") {
		t.Fatalf("expected format error (exit 2), got %v", err)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("invalid format created a report file: %v", err)
	}
}

func TestRunCmd_ConfigReportAndFlagOverride(t *testing.T) {
	configPath, bookPath := writeReportRunbook(t, "true")
	configuredPath := filepath.Join(t.TempDir(), "configured.xml")
	configData := fmt.Sprintf("report:\n  format: junit\n  output: %q\n", configuredPath)
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatal(err)
	}

	root := cmd.NewRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"run", "--config", configPath, bookPath})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(configuredPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte(xml.Header)) || stdout.Len() != 0 {
		t.Fatalf("config report settings not applied: file=%q stdout=%q", content, stdout.String())
	}

	root = cmd.NewRootCmd()
	stdout.Reset()
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"run", "--config", configPath, "--report-format", "json", "--report-out", "", bookPath})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.Total != 1 {
		t.Fatalf("CLI flags did not override config report settings: %v %q", err, stdout.String())
	}
}
