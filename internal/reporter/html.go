package reporter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

//go:embed summary.html.tmpl
var summaryTemplateText string

var summaryTemplate = template.Must(template.New("summary").Funcs(template.FuncMap{
	"elapsed":    formatElapsed,
	"stepLabel":  stepLabel,
	"actualText": actualText,
	"verdict":    verdict,
	"diffItems":  diffItems,
	"json":       compactJSON,
}).Parse(summaryTemplateText))

// HTMLReporter は 1 ファイルで完結するサマリー HTML (summary.html) を出力する。
// 外部のファイル・フォント・CDN は参照しない。
type HTMLReporter struct {
	w io.Writer
}

// NewHTMLReporter は writer に出力する HTMLReporter を生成する。
func NewHTMLReporter(w io.Writer) *HTMLReporter {
	return &HTMLReporter{w: w}
}

// Write は Report をサマリー HTML として出力する。
func (h *HTMLReporter) Write(r *Report) error {
	return summaryTemplate.Execute(h.w, newSummaryView(r))
}

// Close は何もしない (Writer は呼び出し元が管理する)。
func (h *HTMLReporter) Close() error { return nil }

type summaryView struct {
	*Report
	Backends []backendView
	Runbooks []runbookView
}

type backendView struct {
	Name string
	Backend
}

type runbookView struct {
	RunResult
	Steps []stepView
}

type stepView struct {
	StepResult
	Links []evidenceLink
	Diffs []diffView
}

type evidenceLink struct {
	Name string
	Href template.URL
}

type diffView struct {
	DiffSummary
	Link *evidenceLink
}

func newSummaryView(r *Report) summaryView {
	v := summaryView{Report: r}
	names := make([]string, 0, len(r.Backends))
	for k := range r.Backends {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		v.Backends = append(v.Backends, backendView{Name: k, Backend: r.Backends[k]})
	}
	for _, res := range r.Results {
		rb := runbookView{RunResult: res}
		for _, s := range res.Steps {
			sv := stepView{StepResult: s}
			for _, e := range s.Evidence {
				sv.Links = append(sv.Links, evidenceLink{Name: path.Base(e), Href: evidenceHref(r.EvidenceDir, e)})
			}
			for _, d := range s.Diffs {
				dv := diffView{DiffSummary: d}
				if d.File != "" {
					dv.Link = &evidenceLink{Name: path.Base(d.File), Href: evidenceHref(r.EvidenceDir, d.File)}
				}
				sv.Diffs = append(sv.Diffs, dv)
			}
			rb.Steps = append(rb.Steps, sv)
		}
		v.Runbooks = append(v.Runbooks, rb)
	}
	return v
}

// evidenceHref は証跡ファイルへのリンクを作る。証跡のフォルダが実行ごとのフォルダの中なら
// 相対リンク、外 (絶対パス) なら file: の URL にする。
func evidenceHref(dir, file string) template.URL {
	p := file
	if dir != "" {
		p = strings.TrimSuffix(dir, "/") + "/" + file
	}
	abs := strings.HasPrefix(p, "/")
	if len(p) > 2 && p[1] == ':' { // Windows のドライブ (C:/...)
		abs = true
		p = "/" + p
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if i == 1 && abs && strings.HasSuffix(s, ":") {
			continue // ドライブ名の : はそのまま
		}
		segs[i] = url.PathEscape(s)
	}
	escaped := strings.Join(segs, "/")
	if abs {
		return template.URL("file://" + escaped)
	}
	return template.URL(escaped)
}

func formatElapsed(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.1f 秒", float64(ms)/1000)
}

func stepLabel(result string) string {
	switch result {
	case StepSuccess:
		return "成功"
	case StepFailure:
		return "失敗"
	case StepSkipped:
		return "スキップ"
	case StepNotRun:
		return "未実行"
	}
	return result
}

func actualText(actual string) string {
	switch actual {
	case "pass":
		return "成功"
	case "fail":
		return "失敗"
	case "hookFail":
		return "前後処理の失敗"
	case "skipped":
		return "対象外"
	}
	return actual
}

// verdict は runbook の合否 (pass / fail / skip) を返す。CSS のクラス名にも使う。
func verdict(r RunResult) string {
	switch {
	case r.Actual == "skipped":
		return "skip"
	case r.Passed:
		return "pass"
	default:
		return "fail"
	}
}

type diffItem struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
}

func diffItems(d DiffSummary) []diffItem {
	items := make([]diffItem, 0, len(d.Items))
	for _, raw := range d.Items {
		b, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var it diffItem
		if json.Unmarshal(b, &it) == nil {
			items = append(items, it)
		}
	}
	return items
}
