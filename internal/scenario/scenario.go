// Package scenario は runbook のトップレベルにある runnora: ブロックを扱う。
//
// runnora: ブロックはシナリオ固有の情報 (ID、前後処理の SQL、期待する結果、実行してよい環境)
// を持つ。runn はトップレベルの未知のキーを無視するため、runbook はそのまま runn に渡せる。
// ブロックを読むのはトップレベルの runbook だけで、include された runbook のブロックは使わない
// (runn も include 先では BeforeFunc / AfterFunc を呼ばないため)。
package scenario

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// 期待する結果 (runnora.expect) と実際の結果の分類。
const (
	OutcomePass     = "pass"
	OutcomeFail     = "fail"
	OutcomeHookFail = "hookFail"
	// OutcomeSkipped は実行しなかった (envs の対象外) ことを表す。実際の結果だけで使う。
	OutcomeSkipped = "skipped"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Meta は runbook の runnora: ブロック。
type Meta struct {
	ID     string   `yaml:"id"`
	Before []string `yaml:"before"`
	After  []string `yaml:"after"`
	Expect string   `yaml:"expect"`
	Envs   []string `yaml:"envs"`
}

// Runbook は runnora が読み取った runbook の情報。
type Runbook struct {
	// Path は runbook の絶対パス。
	Path string
	// ID はシナリオ ID。runnora: ブロックの id、なければパスから作った ID。
	ID     string
	Desc   string
	Labels []string
	// Meta は runnora: ブロック。ブロックがなければ nil。
	Meta *Meta
	// Text は runbook の本文 (変数参照の検査に使う)。
	Text string
}

// Expect は期待する結果を返す。ブロックがないか expect を省略した場合は pass。
func (r *Runbook) Expect() string {
	if r.Meta == nil || r.Meta.Expect == "" {
		return OutcomePass
	}
	return r.Meta.Expect
}

// AllowedIn は環境 env で実行してよいかを返す。envs を書いていなければすべての環境で実行できる。
func (r *Runbook) AllowedIn(env string) bool {
	if r.Meta == nil || len(r.Meta.Envs) == 0 {
		return true
	}
	for _, e := range r.Meta.Envs {
		if e == env {
			return true
		}
	}
	return false
}

// Read は runbook を読み、runnora: ブロックを取り出す。
// base は ID をパスから作るときの基準ディレクトリ (プロジェクトルートなど)。
func Read(path, base string) (*Runbook, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("scenario: %w", err)
	}
	var head struct {
		Desc    string   `yaml:"desc"`
		Labels  []string `yaml:"labels"`
		Runnora *Meta    `yaml:"runnora"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("scenario: %s: %w", path, err)
	}
	rb := &Runbook{Path: abs, Desc: head.Desc, Labels: head.Labels, Meta: head.Runnora, Text: string(data)}
	if rb.Meta != nil {
		if err := rb.Meta.validate(); err != nil {
			return nil, fmt.Errorf("scenario: %s: runnora: %w", path, err)
		}
		rb.ID = rb.Meta.ID
	} else {
		rb.ID = DeriveID(abs, base)
	}
	return rb, nil
}

func (m *Meta) validate() error {
	var errs []error
	switch {
	case m.ID == "":
		errs = append(errs, errors.New("id がありません"))
	case !idPattern.MatchString(m.ID):
		errs = append(errs, fmt.Errorf("id %q に使えない文字があります (英数字、-、_、. のみ)", m.ID))
	}
	switch m.Expect {
	case "", OutcomePass, OutcomeFail, OutcomeHookFail:
	default:
		errs = append(errs, fmt.Errorf("expect %q は使えません (pass / fail / hookFail)", m.Expect))
	}
	return errors.Join(errs...)
}

// DeriveID は runnora: ブロックのない runbook の ID をパスから作る。
// base からの相対パスから拡張子を除き、区切りを / にしたもの (例: runbooks/demo/x)。
// base の外にある場合はファイル名から作る。
func DeriveID(path, base string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = filepath.Base(path)
	}
	rel = filepath.ToSlash(rel)
	return strings.TrimSuffix(rel, filepath.Ext(rel))
}

// SQLFiles は runnora: ブロックの before / after を、base を基準にした絶対パスで返す。
func (r *Runbook) SQLFiles(base string) (before, after []string) {
	if r.Meta == nil {
		return nil, nil
	}
	return absAll(r.Meta.Before, base), absAll(r.Meta.After, base)
}

func absAll(paths []string, base string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if filepath.IsAbs(p) {
			out = append(out, filepath.Clean(p))
			continue
		}
		out = append(out, filepath.Join(base, filepath.FromSlash(p)))
	}
	return out
}

// CheckDuplicateIDs は同じ ID の runbook がないかを確認する。
func CheckDuplicateIDs(rbs []*Runbook) error {
	seen := map[string]string{}
	var errs []error
	for _, rb := range rbs {
		if prev, ok := seen[rb.ID]; ok {
			errs = append(errs, fmt.Errorf("シナリオ ID %q が重複しています: %s と %s", rb.ID, prev, rb.Path))
			continue
		}
		seen[rb.ID] = rb.Path
	}
	return errors.Join(errs...)
}
