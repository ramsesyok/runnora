package reporter

import (
	"fmt"
	"io"
	"os"
)

// Reporter は実行結果を出力するインターフェース。
//
// このインターフェースにより、出力先（stdout / ファイル）と
// 出力形式（テキスト / 将来的に JSON や JUnit など）を分離できる。
//
// 実装:
//   - TextReporter: テキスト形式で io.Writer に出力 (stdout / ファイルを透過的に扱う)
//   - fileReporter: ファイルを管理するラッパー (Close でファイルを閉じる)
//
// 使い分け:
//   - --report-out 未指定: NewTextReporter(cmd.OutOrStdout()) → stdout へ出力
//   - --report-out 指定:   NewFileReporter(format, path) → ファイルへ出力
type Reporter interface {
	// Write は Report の内容を出力する。
	Write(r *Report) error
	// Close は出力先を閉じる。stdout の場合は何もしない。
	Close() error
}

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
	fmt.Fprintf(t.w, "Runbooks: %d, Passed: %d, Failed: %d\n", r.Total, r.Passed, r.Failed)
	for _, res := range r.Results {
		if !res.Passed {
			fmt.Fprintf(t.w, "  FAIL: %s\n", res.Path)
			if res.Error != "" {
				// エラー詳細を 4 スペースインデントで出力する
				fmt.Fprintf(t.w, "    Error: %s\n", res.Error)
			}
		}
	}
	return nil
}

// Close は何もしない。
// TextReporter は io.Writer を所有しないため、呼び出し元が Writer のライフサイクルを管理する。
// Reporter インターフェースの実装として定義しているが、実際には何もしない。
func (t *TextReporter) Close() error { return nil }

// fileReporter はファイルへ出力する Reporter のラッパー。
//
// 設計: デコレータパターン
//   - f: ファイルの Open/Close を管理
//   - delegate: 実際の書き込みを行う Reporter (TextReporter など)
//
// NewFileReporter が *os.File と TextReporter の両方を生成し、
// Close 時にファイルを閉じる責任を持つ。
type fileReporter struct {
	f        *os.File
	delegate Reporter
}

// NewFileReporter は指定パスにファイルを作成し、format に対応した Reporter を返す。
//
// 現在サポートする format:
//   - "text" またはそれ以外: TextReporter (テキスト形式)
//   - 将来拡張予定: "json", "junit" など
//
// 返り値の Reporter の Close() を呼ぶことでファイルが閉じられる。
// defer rep.Close() を使って確実にファイルを閉じること。
func NewFileReporter(format, path string) (Reporter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("reporter: create %s: %w", path, err)
	}
	var delegate Reporter
	switch format {
	// 将来: case "json": delegate = NewJSONReporter(f)
	default:
		delegate = NewTextReporter(f)
	}
	return &fileReporter{f: f, delegate: delegate}, nil
}

// Write は delegate (TextReporter 等) に処理を委譲する。
func (r *fileReporter) Write(rep *Report) error { return r.delegate.Write(rep) }

// Close はファイルを閉じる。defer で確実に呼ぶこと。
func (r *fileReporter) Close() error { return r.f.Close() }
