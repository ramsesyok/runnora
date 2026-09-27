package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ramsesyok/runnora/internal/config"
)

// legacyConfig は旧形式の config.yaml (runnora v0.3.0 まで)。
type legacyConfig struct {
	App struct {
		Name string `yaml:"name"`
	} `yaml:"app"`
	Oracle struct {
		Driver             string `yaml:"driver"`
		DSN                string `yaml:"dsn"`
		MaxOpenConns       int    `yaml:"max_open_conns"`
		MaxIdleConns       int    `yaml:"max_idle_conns"`
		ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_sec"`
	} `yaml:"oracle"`
	Runn struct {
		Trace        bool   `yaml:"trace"`
		DBRunnerName string `yaml:"db_runner_name"`
	} `yaml:"runn"`
	Hooks struct {
		Common struct {
			Before []string `yaml:"before"`
			After  []string `yaml:"after"`
		} `yaml:"common"`
	} `yaml:"hooks"`
	Generate config.GenerateConfig `yaml:"generate"`
	Report   struct {
		Format string `yaml:"format"`
		Output string `yaml:"output"`
	} `yaml:"report"`
}

// EnvSpec は旧形式の設定ファイル 1 つと、移行後の環境名の対応。
type EnvSpec struct {
	Name string
	// File は Dir からの相対パス。
	File string
}

// environment は移行後の環境 1 つ分。
type environment struct {
	spec   EnvSpec
	cfg    legacyConfig
	vars   map[string]string
	varsOf map[string]string // 変数名 → 追加した理由 (コメント)
}

// DiscoverEnvs は dir にある旧形式の設定ファイルを探し、環境名を決める。
// config.yaml は "default"、config.<名前>.yaml は "<名前>" とする。
func DiscoverEnvs(dir string) ([]EnvSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var specs []EnvSpec
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "config") {
			continue
		}
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		switch {
		case base == "config":
			specs = append(specs, EnvSpec{Name: "default", File: name})
		case strings.HasPrefix(base, "config."):
			specs = append(specs, EnvSpec{Name: strings.TrimPrefix(base, "config."), File: name})
		}
	}
	// config.yaml を先頭 (既定の環境) にする
	sort.SliceStable(specs, func(i, j int) bool { return specs[i].Name == "default" && specs[j].Name != "default" })
	return specs, nil
}

func loadLegacy(path string) (legacyConfig, error) {
	var cfg legacyConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// normalizeHookPath は旧形式のフックのパス (実行時のカレントディレクトリ基準) を
// プロジェクトルート基準の / 区切りのパスにする。スクリプトはプロジェクトのフォルダで
// runnora を実行していた前提。
func normalizeHookPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	return strings.TrimPrefix(p, "./")
}

// renderProject は runnora.yaml の内容を作る。
// コメントを残すため、YAML のライブラリで書き出さずにテキストで組み立てる。
func renderProject(envs []*environment, suites string) string {
	var b strings.Builder
	files := make([]string, 0, len(envs))
	for _, e := range envs {
		files = append(files, e.spec.File)
	}
	fmt.Fprintf(&b, "# runnora-migrate が %s から生成したプロジェクトファイル。\n", strings.Join(files, " / "))
	b.WriteString("# パスはこのファイルのあるディレクトリを基準にする。\n")
	b.WriteString("version: 2\n\n")

	first := envs[0].cfg
	if first.App.Name != "" {
		fmt.Fprintf(&b, "project:\n  name: %s\n\n", q(first.App.Name))
	}
	fmt.Fprintf(&b, "defaults:\n  env: %s\n\n", q(envs[0].spec.Name))

	b.WriteString("environments:\n")
	for _, e := range envs {
		fmt.Fprintf(&b, "  %s:\n", e.spec.Name)
		fmt.Fprintf(&b, "    # %s から移行\n", e.spec.File)
		if len(e.vars) > 0 {
			b.WriteString("    vars:\n")
			for _, k := range sortedKeys(e.vars) {
				line := fmt.Sprintf("      %s: %s", k, q(e.vars[k]))
				if note := e.varsOf[k]; note != "" {
					line += "  # " + note
				}
				b.WriteString(line + "\n")
			}
		}
		o := e.cfg.Oracle
		if o.DSN != "" {
			b.WriteString("    oracle:\n")
			fmt.Fprintf(&b, "      dsn: %s\n", q("${ORACLE_DSN}"))
			writeInt(&b, "      max_open_conns", o.MaxOpenConns)
			writeInt(&b, "      max_idle_conns", o.MaxIdleConns)
			writeInt(&b, "      conn_max_lifetime_sec", o.ConnMaxLifetimeSec)
		}
		h := e.cfg.Hooks.Common
		if len(h.Before) > 0 || len(h.After) > 0 {
			b.WriteString("    hooks:\n")
			writeList(&b, "      before", h.Before)
			writeList(&b, "      after", h.After)
		}
	}
	b.WriteString("\n")

	if suites != "" {
		b.WriteString("# シナリオ対応表の suites から移行\n")
		b.WriteString(suites + "\n")
	} else {
		b.WriteString("# TODO(runnora-migrate): スクリプトで実行していた runbook のまとまりを suites に定義する\n")
		b.WriteString("# suites:\n#   scenarios:\n#     select:\n#       paths: [runbooks/scenarios/*.yml]\n\n")
	}

	if first.Runn.Trace {
		b.WriteString("runn:\n  trace: true\n\n")
	}
	if first.Report.Format != "" || first.Report.Output != "" {
		b.WriteString("report:\n")
		if first.Report.Format != "" {
			fmt.Fprintf(&b, "  format: %s\n", q(first.Report.Format))
		}
		if first.Report.Output != "" {
			fmt.Fprintf(&b, "  output: %s\n", q(first.Report.Output))
		}
		b.WriteString("\n")
	}
	if g := first.Generate; g != (config.GenerateConfig{}) {
		b.WriteString("generate:\n")
		writeStr(&b, "  openapi", g.OpenAPI)
		writeStr(&b, "  out_dir", g.OutDir)
		writeStr(&b, "  case_format", g.CaseFormat)
		writeStr(&b, "  case_style", g.CaseStyle)
		writeStr(&b, "  mode", g.Mode)
		if g.CleanGenerated {
			b.WriteString("  clean_generated: true\n")
		}
		if g.EmitManifest {
			b.WriteString("  emit_manifest: true\n")
		}
		writeStr(&b, "  runner_name", g.RunnerName)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeInt(b *strings.Builder, key string, v int) {
	if v != 0 {
		fmt.Fprintf(b, "%s: %d\n", key, v)
	}
}

func writeStr(b *strings.Builder, key, v string) {
	if v != "" {
		fmt.Fprintf(b, "%s: %s\n", key, q(v))
	}
}

func writeList(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	indent := strings.Repeat(" ", len(key)-len(strings.TrimLeft(key, " ")))
	for _, it := range items {
		fmt.Fprintf(b, "%s  - %s\n", indent, q(normalizeHookPath(it)))
	}
}

// q は文字列を YAML の二重引用符付きスカラーにする (JSON の文字列と同じ書式)。
func q(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
