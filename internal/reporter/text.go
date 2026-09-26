package reporter

import (
	"fmt"
	"io"
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
// サマリー行を出力した後、失敗した runbook の詳細を FAIL: パス形式で出力する。
// 成功した runbook は詳細出力しない (サマリーのカウントのみ)。
func (t *TextReporter) Write(r *Report) error {
	if _, err := fmt.Fprintf(t.w, "Runbooks: %d, Passed: %d, Failed: %d\n", r.Total, r.Passed, r.Failed); err != nil {
		return err
	}
	for _, res := range r.Results {
		if !res.Passed {
			if _, err := fmt.Fprintf(t.w, "  FAIL: %s\n", res.Path); err != nil {
				return err
			}
			if res.Error != "" {
				// エラー詳細を 4 スペースインデントで出力する
				if _, err := fmt.Fprintf(t.w, "    Error: %s\n", res.Error); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Close は何もしない。
// TextReporter は io.Writer を所有しないため、呼び出し元が Writer のライフサイクルを管理する。
// Reporter インターフェースの実装として定義しているが、実際には何もしない。
func (t *TextReporter) Close() error { return nil }
