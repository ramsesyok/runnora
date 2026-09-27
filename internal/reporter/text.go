package reporter

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// TextReporter はテキスト形式でレポートを出力する。
//
// 出力形式:
//
//	Runbooks: 3, Passed: 2, Failed: 1
//	  FAIL: ./runbooks/user_create.yml
//	    Error: assert failed: steps.check.res.status == 200
//
// TextReporter は io.Writer を外部から受け取るため、
// stdout / bytes.Buffer / ファイル など任意の Writer に書き込める。
// これによりテストから bytes.Buffer を渡して出力を検証できる。
type TextReporter struct {
	w io.Writer
}

// NewTextReporter は writer に出力する TextReporter を生成する。
func NewTextReporter(w io.Writer) *TextReporter {
	return &TextReporter{w: w}
}

// Write は Report をテキスト形式で出力する。
//
// 環境やスイートの情報があれば先頭に出力し、サマリー行の後に次を出力する。
//   - 期待どおりでなかった runbook (FAIL:)
//   - 期待どおりに失敗した runbook (PASS: ... (expected ...))
//   - 環境の対象外で実行しなかった runbook (SKIP:)
//
// 期待どおりに成功した runbook は詳細を出力しない (サマリーのカウントのみ)。
func (t *TextReporter) Write(r *Report) error {
	if err := t.writeHeader(r); err != nil {
		return err
	}
	summary := fmt.Sprintf("Runbooks: %d, Passed: %d, Failed: %d", r.Total, r.Passed, r.Failed)
	if r.Skipped > 0 {
		summary += fmt.Sprintf(", Skipped: %d", r.Skipped)
	}
	if _, err := fmt.Fprintln(t.w, summary); err != nil {
		return err
	}
	for _, res := range r.Results {
		var err error
		switch {
		case !res.Passed:
			_, err = fmt.Fprintf(t.w, "  FAIL: %s%s\n", describe(res), mismatch(res))
			if err == nil && res.Error != "" {
				// エラー詳細を 4 スペースインデントで出力する
				_, err = fmt.Fprintf(t.w, "    Error: %s\n", res.Error)
			}
			if err == nil {
				err = t.writeFailedSteps(res)
			}
		case res.Actual == "skipped":
			_, err = fmt.Fprintf(t.w, "  SKIP: %s (この環境は対象外)\n", describe(res))
		case res.Actual != "" && res.Actual != "pass":
			_, err = fmt.Fprintf(t.w, "  PASS: %s (expected %s)\n", describe(res), res.Actual)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// maxFailedSteps は FAIL の下に出す失敗したステップの最大数。
const maxFailedSteps = 5

// writeFailedSteps は失敗したステップのキーとメッセージ (1 行目) を出す。
func (t *TextReporter) writeFailedSteps(res RunResult) error {
	n := 0
	for _, s := range res.Steps {
		if s.Result != StepFailure {
			continue
		}
		if n == maxFailedSteps {
			_, err := fmt.Fprintf(t.w, "    ... (失敗したステップは report.json を参照)\n")
			return err
		}
		n++
		msg, _, _ := strings.Cut(s.Error, "\n")
		if _, err := fmt.Fprintf(t.w, "    Step %s: %s\n", s.Key, msg); err != nil {
			return err
		}
	}
	return nil
}

func (t *TextReporter) writeHeader(r *Report) error {
	var parts []string
	if r.Project != "" {
		parts = append(parts, "Project: "+r.Project)
	}
	if r.Env != nil {
		env := "Env: " + r.Env.Name
		if r.Env.Description != "" {
			env += " (" + r.Env.Description + ")"
		}
		parts = append(parts, env)
	}
	if r.Suite != "" {
		parts = append(parts, "Suite: "+r.Suite)
	}
	if len(parts) > 0 {
		if _, err := fmt.Fprintln(t.w, strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	if len(r.Backends) > 0 {
		names := make([]string, 0, len(r.Backends))
		for k := range r.Backends {
			names = append(names, k)
		}
		sort.Strings(names)
		items := make([]string, 0, len(names))
		for _, k := range names {
			b := r.Backends[k]
			item := k + "=" + b.Mode
			if b.Note != "" {
				item += " (" + b.Note + ")"
			}
			items = append(items, item)
		}
		if _, err := fmt.Fprintf(t.w, "Backends: %s\n", strings.Join(items, ", ")); err != nil {
			return err
		}
	}
	if r.Env != nil && len(r.Env.Overrides) > 0 {
		if _, err := fmt.Fprintf(t.w, "Env overrides: %s\n", strings.Join(r.Env.Overrides, ", ")); err != nil {
			return err
		}
	}
	return nil
}

// describe は runbook を "ID (パス)" の形で表す。ID がなければパスだけ。
func describe(res RunResult) string {
	if res.ID != "" && res.ID != res.Path {
		return res.ID + " (" + res.Path + ")"
	}
	return res.Path
}

// mismatch は期待と実際が異なるときに " [expected X, got Y]" を返す。
func mismatch(res RunResult) string {
	if res.Expect == "" || res.Actual == "" || res.Expect == "pass" {
		return ""
	}
	return fmt.Sprintf(" [expected %s, got %s]", res.Expect, res.Actual)
}

// Close は何もしない。
// TextReporter は io.Writer を所有しないため、呼び出し元が Writer のライフサイクルを管理する。
// Reporter インターフェースの実装として定義しているが、実際には何もしない。
func (t *TextReporter) Close() error { return nil }
