// Package project は runnora のプロジェクトファイル (runnora.yaml) を扱う。
//
// runnora.yaml はテストプロジェクト全体の設定を持つ。
//   - environments: 接続先や DB、共通の前後処理 SQL を環境ごとに定義する
//   - suites:       実行する runbook の選び方を名前付きで定義する
//   - runn / report / generate: 各コマンドの既定値
//
// ファイル中のパスはすべてプロジェクトルート (runnora.yaml のあるディレクトリ) を基準にする。
// 詳細は docs/design/format-v2.md を参照。
package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ramsesyok/runnora/internal/config"
)

// FileName はプロジェクトファイルの名前。
const FileName = "runnora.yaml"

// Version は現在の形式のバージョン。
const Version = 2

// MigrateHint は旧形式を検出したときに案内する文言。
const MigrateHint = "runnora-migrate で runnora.yaml (version: 2) に移行してください"

// Project は runnora.yaml の内容を表す。
type Project struct {
	Version      int                     `yaml:"version"`
	Info         Info                    `yaml:"project"`
	Defaults     Defaults                `yaml:"defaults"`
	Environments map[string]*Environment `yaml:"environments"`
	Suites       map[string]*Suite       `yaml:"suites"`
	Runn         RunnSettings            `yaml:"runn"`
	Report       ReportSettings          `yaml:"report"`
	Generate     config.GenerateConfig   `yaml:"generate"`

	// Path は読み込んだ runnora.yaml の絶対パス。
	Path string `yaml:"-"`
	// Root はプロジェクトルート (runnora.yaml のあるディレクトリ) の絶対パス。
	Root string `yaml:"-"`
}

// Info はプロジェクトの説明情報。
type Info struct {
	Name string `yaml:"name"`
}

// Defaults は CLI で省略した値の既定値。
type Defaults struct {
	Env string `yaml:"env"`
}

// Environment は 1 つの実行環境の定義。
type Environment struct {
	Description string              `yaml:"description"`
	Vars        map[string]string   `yaml:"vars"`
	Oracle      *OracleSettings     `yaml:"oracle"`
	Hooks       Hooks               `yaml:"hooks"`
	Backends    map[string]*Backend `yaml:"backends"`
}

// OracleSettings はフック用の Oracle 接続設定。値に ${VAR} を書ける。
type OracleSettings struct {
	DSN                string `yaml:"dsn"`
	MaxOpenConns       int    `yaml:"max_open_conns"`
	MaxIdleConns       int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_sec"`
}

// Hooks は環境の全シナリオに共通する前後処理の SQL ファイル。
type Hooks struct {
	Before []string `yaml:"before"`
	After  []string `yaml:"after"`
}

// Backend はテスト対象の裏にあるサービスの扱い。宣言と記録だけで動作は変えない。
type Backend struct {
	Mode string `yaml:"mode" json:"mode"`
	Note string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Suite は実行する runbook の選び方。
type Suite struct {
	Env    string            `yaml:"env"`
	Select Selection         `yaml:"select"`
	Vars   map[string]string `yaml:"vars"`
}

// Selection はスイートの対象 runbook の条件。
type Selection struct {
	Paths  []string `yaml:"paths"`
	Labels []string `yaml:"labels"`
	IDs    []string `yaml:"ids"`
}

// RunnSettings は runn に渡す設定。
type RunnSettings struct {
	Scopes []string `yaml:"scopes"`
	Trace  bool     `yaml:"trace"`
}

// ReportSettings はレポートの既定値。
type ReportSettings struct {
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// LegacyError は旧形式の設定を検出したことを表す。
type LegacyError struct {
	Reason string
}

func (e *LegacyError) Error() string {
	return fmt.Sprintf("%s。%s", e.Reason, MigrateHint)
}

// Find は使う runnora.yaml のパスを返す。
//
// explicit が空でなければそのパスを使う (存在しなければエラー)。
// 空の場合は dir から親ディレクトリへ向かって runnora.yaml を探す。
// 見つからなければ "" を返す (プロジェクトなしとして動く)。
func Find(explicit, dir string) (string, error) {
	if explicit != "" {
		p, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("project: %w", err)
		}
		return p, nil
	}
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(d, FileName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", nil
		}
		d = parent
	}
}

// Load は runnora.yaml を読み込み、構造を検証する。
// 変数の展開はここでは行わない (Resolve で行う)。
func Load(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("project: %s: %w", path, err)
	}
	p.Path = abs
	p.Root = filepath.Dir(abs)
	return p, nil
}

// Parse は runnora.yaml の内容を解析して検証する。
func Parse(data []byte) (*Project, error) {
	var p Project
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		// version を持たない旧形式 (app: / oracle: / hooks: など) は KnownFields で失敗する。
		// version だけを読み直して、旧形式なら移行を案内する。
		var head struct {
			Version int `yaml:"version"`
		}
		if yaml.Unmarshal(data, &head) == nil && head.Version != Version {
			return nil, legacyVersionError(head.Version)
		}
		return nil, err
	}
	if p.Version != Version {
		return nil, legacyVersionError(p.Version)
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

func legacyVersionError(v int) error {
	if v == 0 {
		return &LegacyError{Reason: "version がありません (旧形式の設定ファイルです)"}
	}
	return &LegacyError{Reason: fmt.Sprintf("version %d には対応していません (対応: %d)", v, Version)}
}

// validate は環境とスイートの参照が正しいかを確認する。
func (p *Project) validate() error {
	var errs []error
	if p.Defaults.Env != "" {
		if _, ok := p.Environments[p.Defaults.Env]; !ok {
			errs = append(errs, fmt.Errorf("defaults.env: 環境 %q が environments にありません", p.Defaults.Env))
		}
	}
	for _, name := range sortedKeys(p.Environments) {
		env := p.Environments[name]
		if env == nil {
			p.Environments[name] = &Environment{}
			continue
		}
		for bname, b := range env.Backends {
			if b == nil {
				errs = append(errs, fmt.Errorf("environments.%s.backends.%s: mode がありません", name, bname))
				continue
			}
			switch b.Mode {
			case "stub", "mock", "real":
			default:
				errs = append(errs, fmt.Errorf("environments.%s.backends.%s.mode: %q は使えません (stub / mock / real)", name, bname, b.Mode))
			}
		}
	}
	for _, name := range sortedKeys(p.Suites) {
		s := p.Suites[name]
		if s == nil || len(s.Select.Paths) == 0 {
			errs = append(errs, fmt.Errorf("suites.%s.select.paths: 対象の runbook の glob を指定してください", name))
			continue
		}
		if s.Env != "" {
			if _, ok := p.Environments[s.Env]; !ok {
				errs = append(errs, fmt.Errorf("suites.%s.env: 環境 %q が environments にありません", name, s.Env))
			}
		}
	}
	return errors.Join(errs...)
}

// EnvironmentNames は定義済みの環境名を名前順で返す。
func (p *Project) EnvironmentNames() []string {
	return sortedKeys(p.Environments)
}

// SuiteNames は定義済みのスイート名を名前順で返す。
func (p *Project) SuiteNames() []string {
	return sortedKeys(p.Suites)
}

// SelectEnv は使う環境名を決める。
//
// 優先順: スイートの env → --env → defaults.env → 環境が 1 つだけならそれ。
// スイートの env と --env が両方指定されて食い違う場合はエラーにする。
// 環境が 1 つも定義されていなければ "" を返す (環境なしで動く)。
func (p *Project) SelectEnv(flagEnv, suiteName string) (string, error) {
	suiteEnv := ""
	if suiteName != "" {
		s, ok := p.Suites[suiteName]
		if !ok {
			return "", fmt.Errorf("スイート %q が runnora.yaml にありません (定義済み: %v)", suiteName, p.SuiteNames())
		}
		suiteEnv = s.Env
	}
	if flagEnv != "" {
		if _, ok := p.Environments[flagEnv]; !ok {
			return "", fmt.Errorf("環境 %q が runnora.yaml にありません (定義済み: %v)", flagEnv, p.EnvironmentNames())
		}
		if suiteEnv != "" && suiteEnv != flagEnv {
			return "", fmt.Errorf("スイート %q は環境 %q で実行する定義ですが、--env %s が指定されました", suiteName, suiteEnv, flagEnv)
		}
		return flagEnv, nil
	}
	if suiteEnv != "" {
		return suiteEnv, nil
	}
	if p.Defaults.Env != "" {
		return p.Defaults.Env, nil
	}
	switch len(p.Environments) {
	case 0:
		return "", nil
	case 1:
		return p.EnvironmentNames()[0], nil
	default:
		return "", fmt.Errorf("環境が複数あります。--env か runnora.yaml の defaults.env で指定してください (定義済み: %v)", p.EnvironmentNames())
	}
}

// Abs はプロジェクトルート基準の相対パスを絶対パスにする。
func (p *Project) Abs(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(p.Root, filepath.FromSlash(path))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
