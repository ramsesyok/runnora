package project

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ramsesyok/runnora/internal/config"
)

// Oracle 接続プールの既定値。runnora.yaml で 0 または省略したときに使う。
const (
	defaultMaxOpenConns       = 10
	defaultMaxIdleConns       = 2
	defaultConnMaxLifetimeSec = 300
)

// Lookup は OS の環境変数を引く関数 (os.LookupEnv と同じ形)。テストで差し替える。
type Lookup func(name string) (string, bool)

// Resolved は選んだ環境とスイートについて、変数をすべて展開した結果。
type Resolved struct {
	// Name は環境名。環境を使わない場合は ""。
	Name        string
	Description string
	// Vars は runbook に渡す変数表 (プロセスの環境変数に設定する)。
	Vars map[string]string
	// Overrides は runnora.yaml の vars に書いた値を OS の環境変数か --var が上書きした変数の名前。
	Overrides []string
	// Oracle はフック用の接続設定。HasOracle が false なら未設定。
	Oracle    config.OracleConfig
	HasOracle bool
	// Before / After は環境の共通フック (絶対パス)。
	Before []string
	After  []string
	// Backends は裏のサービスの扱い (記録用)。
	Backends map[string]Backend
	// ReportOutput は report.output を展開した値。
	ReportOutput string
}

// UndefinedError は既定値のない未定義の変数を参照していることを表す。
type UndefinedError struct {
	// Where は参照している場所 (例: "environments.unit.vars.API_URL")。
	Where string
	Names []string
}

func (e *UndefinedError) Error() string {
	return fmt.Sprintf("%s: 未定義の変数 %s を参照しています (runnora.yaml の vars、OS の環境変数、--var のどれかで定義するか、${NAME:-既定値} と書いてください)",
		e.Where, strings.Join(e.Names, ", "))
}

// Resolve は環境 envName とスイート suiteName の変数を展開する。
//
// 変数の優先順位 (上が優先):
//  1. cli (--var)
//  2. OS の環境変数 (lookup)
//  3. スイートの vars
//  4. 環境の vars
//
// envName が "" のときは環境なし (vars はスイートと --var だけ) として扱う。
func (p *Project) Resolve(envName, suiteName string, cli map[string]string, lookup Lookup) (*Resolved, error) {
	var env *Environment
	if envName != "" {
		var ok bool
		env, ok = p.Environments[envName]
		if !ok {
			return nil, fmt.Errorf("環境 %q が runnora.yaml にありません (定義済み: %v)", envName, p.EnvironmentNames())
		}
	} else {
		env = &Environment{}
	}
	var suite *Suite
	if suiteName != "" {
		var ok bool
		suite, ok = p.Suites[suiteName]
		if !ok {
			return nil, fmt.Errorf("スイート %q が runnora.yaml にありません (定義済み: %v)", suiteName, p.SuiteNames())
		}
	}

	// 段階 1: runnora.yaml の vars を集め (スイートが環境より優先)、値の中の ${...} を
	// --var と OS の環境変数で展開する。--var か OS に同名の値があるものは yaml の値を使わない。
	declared := map[string]string{}
	where := map[string]string{}
	for k, v := range env.Vars {
		declared[k] = v
		where[k] = fmt.Sprintf("environments.%s.vars.%s", envName, k)
	}
	if suite != nil {
		for k, v := range suite.Vars {
			declared[k] = v
			where[k] = fmt.Sprintf("suites.%s.vars.%s", suiteName, k)
		}
	}
	outer := func(name string) (string, bool) {
		if v, ok := cli[name]; ok {
			return v, true
		}
		return lookup(name)
	}

	res := &Resolved{Name: envName, Description: env.Description, Vars: map[string]string{}}
	var errs []error
	for _, k := range sortedKeys(declared) {
		if v, ok := outer(k); ok {
			res.Vars[k] = v
			res.Overrides = append(res.Overrides, k)
			continue
		}
		v, undefined := Expand(declared[k], outer)
		if len(undefined) > 0 {
			errs = append(errs, &UndefinedError{Where: where[k], Names: undefined})
			continue
		}
		res.Vars[k] = v
	}
	for k, v := range cli {
		if _, ok := declared[k]; !ok {
			res.Vars[k] = v
		}
	}

	// 段階 2: 変数表で oracle / hooks / report の値を展開する。
	table := func(name string) (string, bool) {
		if v, ok := res.Vars[name]; ok {
			return v, true
		}
		return lookup(name)
	}
	expand := func(value, at string) string {
		v, undefined := Expand(value, table)
		if len(undefined) > 0 {
			errs = append(errs, &UndefinedError{Where: at, Names: undefined})
		}
		return v
	}
	envPath := "environments." + envName
	if env.Oracle != nil {
		res.HasOracle = true
		res.Oracle = config.OracleConfig{
			Driver:             "oracle",
			DSN:                expand(env.Oracle.DSN, envPath+".oracle.dsn"),
			MaxOpenConns:       orDefault(env.Oracle.MaxOpenConns, defaultMaxOpenConns),
			MaxIdleConns:       orDefault(env.Oracle.MaxIdleConns, defaultMaxIdleConns),
			ConnMaxLifetimeSec: orDefault(env.Oracle.ConnMaxLifetimeSec, defaultConnMaxLifetimeSec),
		}
	}
	for i, f := range env.Hooks.Before {
		res.Before = append(res.Before, p.Abs(expand(f, fmt.Sprintf("%s.hooks.before[%d]", envPath, i))))
	}
	for i, f := range env.Hooks.After {
		res.After = append(res.After, p.Abs(expand(f, fmt.Sprintf("%s.hooks.after[%d]", envPath, i))))
	}
	if p.Report.Output != "" {
		res.ReportOutput = expand(p.Report.Output, "report.output")
	}
	if len(env.Backends) > 0 {
		res.Backends = map[string]Backend{}
		for k, b := range env.Backends {
			res.Backends[k] = *b
		}
	}
	sort.Strings(res.Overrides)

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return res, nil
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// varPattern は ${NAME} / ${NAME:-既定値} などの変数参照に一致する。
// 1: 変数名、2: 名前の後ろの演算子と値 (":-default" など)。
var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)([^}]*)\}`)

// Expand は s の中の ${NAME} と ${NAME:-既定値} を lookup で展開する。
// 値がなく既定値もない変数の名前を undefined として返す (その箇所は空文字にする)。
func Expand(s string, lookup Lookup) (string, []string) {
	var undefined []string
	out := varPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := varPattern.FindStringSubmatch(m)
		name, op := sub[1], sub[2]
		if v, ok := lookup(name); ok {
			if v != "" || !strings.HasPrefix(op, ":-") {
				return v
			}
		}
		if def, ok := defaultValue(op); ok {
			return def
		}
		undefined = appendUnique(undefined, name)
		return ""
	})
	return out, undefined
}

// Ref は変数参照 1 つ分の情報。
type Ref struct {
	Name       string
	HasDefault bool
}

// Refs は s の中の変数参照を出現順 (重複なし) で返す。
func Refs(s string) []Ref {
	var refs []Ref
	seen := map[string]bool{}
	for _, sub := range varPattern.FindAllStringSubmatch(s, -1) {
		if seen[sub[1]] {
			continue
		}
		seen[sub[1]] = true
		_, hasDefault := defaultValue(sub[2])
		refs = append(refs, Ref{Name: sub[1], HasDefault: hasDefault})
	}
	return refs
}

// defaultValue は ":-値" / "-値" 形式の既定値を返す。
func defaultValue(op string) (string, bool) {
	switch {
	case strings.HasPrefix(op, ":-"):
		return op[2:], true
	case strings.HasPrefix(op, "-"):
		return op[1:], true
	}
	return "", false
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
