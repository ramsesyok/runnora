package reporter

import (
	"fmt"
	"io"
	"os"
)

// Reporter は実行結果を出力するインターフェース。
type Reporter interface {
	Write(r *Report) error
	Close() error
}

// TextReporter はテキスト形式でレポートを出力する。
type TextReporter struct {
	w io.Writer
}

// NewTextReporter は writer に出力する TextReporter を生成する。
func NewTextReporter(w io.Writer) *TextReporter {
	return &TextReporter{w: w}
}

// Write は Report をテキスト形式で出力する。
func (t *TextReporter) Write(r *Report) error {
	fmt.Fprintf(t.w, "Runbooks: %d, Passed: %d, Failed: %d\n", r.Total, r.Passed, r.Failed)
	for _, res := range r.Results {
		if !res.Passed {
			fmt.Fprintf(t.w, "  FAIL: %s\n", res.Path)
			if res.Error != "" {
				fmt.Fprintf(t.w, "    Error: %s\n", res.Error)
			}
		}
	}
	return nil
}

// Close は何もしない（TextReporter は io.Writer を所有しない）。
func (t *TextReporter) Close() error { return nil }

// fileReporter はファイルへ出力する Reporter のラッパー。
type fileReporter struct {
	f        *os.File
	delegate Reporter
}

// NewFileReporter は指定パスにファイルを作成し、format に対応した Reporter を返す。
func NewFileReporter(format, path string) (Reporter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("reporter: create %s: %w", path, err)
	}
	var delegate Reporter
	switch format {
	default:
		delegate = NewTextReporter(f)
	}
	return &fileReporter{f: f, delegate: delegate}, nil
}

func (r *fileReporter) Write(rep *Report) error { return r.delegate.Write(rep) }
func (r *fileReporter) Close() error            { return r.f.Close() }
