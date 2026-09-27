package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/scenario"
)

// diagnostic は validate の検査結果 1 件。
type diagnostic struct {
	Severity string `json:"severity"` // error / warning
	Where    string `json:"where"`
	Message  string `json:"message"`
}

// validateResult は validate の結果全体 (--format json の出力)。
type validateResult struct {
	Project   string       `json:"project"`
	Env       string       `json:"env,omitempty"`
	Overrides []string     `json:"overrides,omitempty"`
	Runbooks  int          `json:"runbooks"`
	Errors    []diagnostic `json:"errors"`
	Warnings  []diagnostic `json:"warnings"`
}

func (r *validateResult) errorf(where, format string, args ...any) {
	r.Errors = append(r.Errors, diagnostic{Severity: "error", Where: where, Message: fmt.Sprintf(format, args...)})
}

func (r *validateResult) warnf(where, format string, args ...any) {
	r.Warnings = append(r.Warnings, diagnostic{Severity: "warning", Where: where, Message: fmt.Sprintf(format, args...)})
}

// newValidateCmd は "runnora validate" サブコマンドを生成する。
//
// runbook を実行せずに、runnora.yaml とスイートが選ぶ runbook を検査する。
//   - runnora.yaml の構文、version、環境・スイートの参照
//   - 変数の未定義 (選んだ環境で展開する。include 先は警告)
//   - 前後処理の SQL ファイルの存在
//   - runnora: ブロックの内容、シナリオ ID の重複、スイートの ids に見つからない ID
//
// エラーがあれば終了コード 2 を返す。
func newValidateCmd() *cobra.Command {
	var (
		projectPath string
		envName     string
		format      string
	)

	cmd := &cobra.Command{
		Use:   "validate [options] [runbook...]",
		Short: "runnora.yaml と runbook を実行せずに検査する",
		Long: `runnora.yaml と、スイートが選ぶ runbook (引数を指定した場合はその runbook) を実行せずに検査する。

変数は --env の環境 (省略時は defaults.env) で展開して確かめる。
環境を固定したスイートは、その環境が選んだ環境と同じ場合だけ変数を確かめる。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 引数の解析が済んだ後のエラーでは使い方を表示しない (エラーが読みにくくなるため)
			cmd.SilenceUsage = true
			if format != "" && format != "text" && format != "json" {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("unsupported format %q (supported: text, json)", format)}
			}
			res, err := runValidate(projectPath, envName, args)
			if err != nil {
				return err
			}
			if format == "json" {
				if res.Errors == nil {
					res.Errors = []diagnostic{}
				}
				if res.Warnings == nil {
					res.Warnings = []diagnostic{}
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					return err
				}
			} else {
				writeValidateText(cmd.OutOrStdout(), res)
			}
			if len(res.Errors) > 0 {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("validate: %d error(s)", len(res.Errors))}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&projectPath, "project", "", "runnora.yaml のパス (省略時はカレントディレクトリから親へ探す)")
	cmd.Flags().StringVar(&envName, "env", "", "変数を展開して確かめる環境 (省略時は defaults.env)")
	cmd.Flags().StringVar(&format, "format", "text", "出力形式 (text|json)")

	return cmd
}

func runValidate(projectPath, envFlag string, args []string) (*validateResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	path, err := project.Find(projectPath, cwd)
	if err != nil {
		return nil, &app.AppError{ExitCode: 2, Cause: err}
	}
	if path == "" {
		return nil, &app.AppError{ExitCode: 2, Cause: fmt.Errorf("runnora.yaml が見つかりません (runnora init で作成できます)")}
	}
	res := &validateResult{Project: path}
	p, err := project.Load(path)
	if err != nil {
		res.errorf(path, "%v", err)
		return res, nil
	}

	envName, err := p.SelectEnv(envFlag, "")
	if err != nil {
		res.errorf("--env", "%v", err)
		return res, nil
	}
	res.Env = envName
	env, err := p.Resolve(envName, "", nil, os.LookupEnv)
	if err != nil {
		addJoined(res, "runnora.yaml", err)
		env = nil
	} else {
		res.Overrides = env.Overrides
		for _, f := range append(append([]string{}, env.Before...), env.After...) {
			if _, err := os.Stat(f); err != nil {
				res.errorf("environments."+envName+".hooks", "SQL ファイルがありません: %s", f)
			}
		}
	}

	// 検査する runbook を集める。引数があればそれ、なければ全スイートが選ぶもの。
	type selected struct {
		rb   *scenario.Runbook
		vars map[string]string
	}
	var targets []selected
	index := map[string]int{}
	add := func(rb *scenario.Runbook, vars map[string]string) {
		if i, ok := index[rb.Path]; ok {
			// 複数のスイートが選ぶ runbook は、変数を確かめられるスイートの変数表を使う。
			if targets[i].vars == nil && vars != nil {
				targets[i].vars = vars
			}
			return
		}
		index[rb.Path] = len(targets)
		targets = append(targets, selected{rb: rb, vars: vars})
	}
	var envVars map[string]string
	if env != nil {
		envVars = env.Vars
	}
	if len(args) > 0 {
		rbs, err := scenario.FromArgs(args, p.Root)
		if err != nil {
			res.errorf("runbook", "%v", err)
		}
		for _, rb := range rbs {
			add(rb, envVars)
		}
	} else {
		for _, name := range p.SuiteNames() {
			s := p.Suites[name]
			rbs, err := scenario.Select(p.Root, s.Select)
			if err != nil {
				addJoined(res, "suites."+name, err)
				continue
			}
			if len(rbs) == 0 {
				res.warnf("suites."+name, "条件に合う runbook がありません (runnora: ブロックを持つ runbook だけが対象です)")
			}
			// 変数はスイートの環境が選んだ環境と同じ場合だけ確かめる。
			var vars map[string]string
			if env != nil && (s.Env == "" || s.Env == envName) {
				sr, err := p.Resolve(envName, name, nil, os.LookupEnv)
				if err != nil {
					addJoined(res, "suites."+name, err)
				} else {
					vars = sr.Vars
				}
			}
			for _, rb := range rbs {
				add(rb, vars)
			}
		}
	}
	res.Runbooks = len(targets)

	var all []*scenario.Runbook
	for _, t := range targets {
		all = append(all, t.rb)
	}
	if err := scenario.CheckDuplicateIDs(all); err != nil {
		addJoined(res, "runnora.id", err)
	}

	for _, t := range targets {
		before, after := t.rb.SQLFiles(p.Root)
		for _, f := range append(before, after...) {
			if _, err := os.Stat(f); err != nil {
				res.errorf(t.rb.Path, "runnora: ブロックの SQL ファイルがありません: %s", f)
			}
		}
		if t.rb.Meta != nil {
			for _, e := range t.rb.Meta.Envs {
				if _, ok := p.Environments[e]; !ok {
					res.errorf(t.rb.Path, "runnora.envs: 環境 %q が runnora.yaml にありません", e)
				}
			}
		}
		if t.vars == nil {
			// 変数表がない (環境を展開できなかった、または別の環境に固定されたスイート) 場合は確かめない。
			continue
		}
		if names := undefinedIn(t.rb.Text, t.vars); len(names) > 0 {
			res.errorf(t.rb.Path, "未定義の変数を参照しています: %s", strings.Join(names, ", "))
		}
		checkIncludes(res, t.rb, t.vars, map[string]bool{t.rb.Path: true})
	}
	return res, nil
}

// checkIncludes は include 先の runbook が参照する未定義の変数を警告する (再帰的にたどる)。
func checkIncludes(res *validateResult, rb *scenario.Runbook, vars map[string]string, visited map[string]bool) {
	for _, inc := range scenario.Includes(rb) {
		if visited[inc] {
			continue
		}
		visited[inc] = true
		child, err := scenario.Read(inc, "")
		if err != nil {
			res.warnf(rb.Path, "include 先を読めません: %v", err)
			continue
		}
		if names := undefinedIn(child.Text, vars); len(names) > 0 {
			res.warnf(inc, "未定義の変数を参照しています: %s (include 元: %s)", strings.Join(names, ", "), rb.Path)
		}
		checkIncludes(res, child, vars, visited)
	}
}

// addJoined は errors.Join でまとめたエラーを 1 件ずつ登録する。
func addJoined(res *validateResult, where string, err error) {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		for _, e := range joined.Unwrap() {
			addJoined(res, where, e)
		}
		return
	}
	for _, line := range strings.Split(err.Error(), "\n") {
		if line != "" {
			res.errorf(where, "%s", line)
		}
	}
}

func writeValidateText(w io.Writer, res *validateResult) {
	fmt.Fprintf(w, "project: %s\n", res.Project)
	if res.Env != "" {
		fmt.Fprintf(w, "env: %s\n", res.Env)
	}
	if len(res.Overrides) > 0 {
		fmt.Fprintf(w, "env override: %s\n", strings.Join(res.Overrides, ", "))
	}
	diags := append(append([]diagnostic{}, res.Errors...), res.Warnings...)
	sort.SliceStable(diags, func(i, j int) bool { return diags[i].Severity < diags[j].Severity })
	for _, d := range diags {
		fmt.Fprintf(w, "%s: %s: %s\n", d.Severity, d.Where, d.Message)
	}
	if len(res.Errors) == 0 && len(res.Warnings) == 0 {
		fmt.Fprintf(w, "OK: %d runbook(s), no issues found\n", res.Runbooks)
		return
	}
	fmt.Fprintf(w, "%d error(s), %d warning(s) in %d runbook(s)\n", len(res.Errors), len(res.Warnings), res.Runbooks)
}
