package reporter

import (
	"encoding/json"
	"io"
)

// JSONReporter は runbook ごとの結果を機械処理しやすい JSON で出力する。
type JSONReporter struct{ w io.Writer }

func NewJSONReporter(w io.Writer) *JSONReporter { return &JSONReporter{w: w} }

func (j *JSONReporter) Write(r *Report) error {
	report := *r
	if report.Results == nil {
		report.Results = []RunResult{}
	}
	enc := json.NewEncoder(j.w)
	enc.SetIndent("", "  ")
	return enc.Encode(&report)
}

func (j *JSONReporter) Close() error { return nil }
