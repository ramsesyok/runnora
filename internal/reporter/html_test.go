package reporter_test

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/reporter"
)

func htmlReport() *reporter.Report {
	return &reporter.Report{
		SchemaVersion: reporter.SchemaVersion, Runnora: "v0.9.0", StartedAt: "2026-09-27T15:30:12+09:00", ElapsedMs: 2345,
		Project: "sample", Suite: "scenarios", EvidenceDir: "evidence",
		Env:      &reporter.EnvInfo{Name: "local", Description: "手元のモック", Overrides: []string{"API_URL"}},
		Backends: map[string]reporter.Backend{"oracle": {Mode: "real", Note: "検証 DB"}},
		Total:    3, Passed: 1, Failed: 1, Skipped: 1,
		Results: []reporter.RunResult{
			{ID: "OK-1", Desc: "正常系", Path: "runbooks/ok.yml", Expect: "pass", Actual: "pass", Passed: true,
				Steps: []reporter.StepResult{{Key: "get", Index: 1, Result: reporter.StepSuccess, Evidence: []string{"OK-1/01-get.json"}}}},
			{ID: "NG-1", Desc: "<金額>の確認", Path: "runbooks/ng.yml", Expect: "pass", Actual: "fail", Error: "assert failed",
				Hooks: []reporter.HookResult{
					{Phase: "before", File: "sql/setup.sql", OK: true},
					{Phase: "after", File: "sql/cleanup.sql", OK: false, Error: "ORA-00942"},
				},
				Steps: []reporter.StepResult{
					{Key: "calc", Index: 1, Result: reporter.StepFailure, Error: "diffEps failed",
						Evidence: []string{"NG-1/01-calc.json", "NG-1/01-calc[1].json"},
						Diffs: []reporter.DiffSummary{{Differences: 7, File: "NG-1/01-calc.diff.json", Items: []any{
							map[string]any{"path": ".amount", "kind": "numberOutOfTolerance", "expected": 100, "actual": 101.5},
						}}}},
					{Key: "after", Index: 2, Result: reporter.StepNotRun},
				}},
			{ID: "SK-1", Path: "runbooks/skip.yml", Actual: "skipped", Passed: true},
		},
	}
}

func TestHTMLReporter_Summary(t *testing.T) {
	var buf bytes.Buffer
	if err := reporter.NewHTMLReporter(&buf).Write(htmlReport()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`<meta charset="utf-8">`,
		"sample", "local（手元のモック）", "上書きされた変数: API_URL", "oracle = real（検証 DB）", "2.3 秒", "v0.9.0",
		`<td class="has-fail">1</td>`,
		`<details class="rb rb-fail" data-order="1">`,
		"&lt;金額&gt;の確認", // エスケープする
		"ORA-00942", `class="hook-fail"`,
		"diffEps: 差分 7 件", `href="evidence/NG-1/01-calc.diff.json"`, ".amount", "numberOutOfTolerance", "101.5", "先頭 1 件のみ表示",
		`href="evidence/NG-1/01-calc%5B1%5D.json"`,
		"未実行", "対象外",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary.html should contain %q", want)
		}
	}
}

// 閉域環境でも開けるよう、外部のファイル・フォント・CDN を参照しない。
func TestHTMLReporter_SelfContained(t *testing.T) {
	var buf bytes.Buffer
	if err := reporter.NewHTMLReporter(&buf).Write(htmlReport()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, bad := range []string{"http:", "https:", "src=", "<link", "@import", "url("} {
		if strings.Contains(out, bad) {
			t.Errorf("summary.html should not contain %q", bad)
		}
	}
	for _, href := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(out, -1) {
		if !strings.HasPrefix(href[1], "evidence/") {
			t.Errorf("unexpected link %q", href[1])
		}
	}
}

func TestHTMLReporter_EvidenceOutsideRunFolder(t *testing.T) {
	tests := []struct {
		dir, want string
	}{
		{dir: "/var/ev dir", want: `href="file:///var/ev%20dir/OK-1/01-get.json"`},
		{dir: "C:/work/ev", want: `href="file:///C:/work/ev/OK-1/01-get.json"`},
		{dir: "../shared/ev", want: `href="../shared/ev/OK-1/01-get.json"`},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			rep := htmlReport()
			rep.EvidenceDir = tt.dir
			var buf bytes.Buffer
			if err := reporter.NewHTMLReporter(&buf).Write(rep); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("want %s", tt.want)
			}
		})
	}
}

func TestHTMLReporter_PropagatesWriteError(t *testing.T) {
	if err := reporter.NewHTMLReporter(errorWriter{err: errors.New("write failed")}).Write(htmlReport()); err == nil {
		t.Fatal("want error")
	}
}
