// Package app は runnora のコアロジックを提供する。
//
// 主な責務:
//   - 環境の変数の設定と Oracle DB 接続の確立
//   - runbook ごとの before/after フックの実行 (SQL/PL/SQL ファイル)
//   - runn を使った runbook の実行と、期待する結果 (runnora.expect) との照合
//   - 終了コードを持つエラー (AppError) の生成
//
// runnora.yaml の読み込みと実行対象の選択は cmd 層が行い、結果を Plan として渡す。
//
// 依存関係:
//
//	app.Runner
//	  ├── Plan (cmd 層が project / scenario から組み立てた実行内容)
//	  ├── oracle.Executor (DB 操作の抽象化 — テスト時は stub に差し替え)
//	  ├── hook.RunBefore / hook.RunAfter (フック実行)
//	  ├── runn.Load / runn.RunN (runbook 実行エンジン)
//	  └── reporter.Report (結果集計)
//
// テスト戦略:
//   - ExecutorFactory を DI することで Oracle 不要のテストを実現
//   - runn の HTTP シナリオは httptest.Server で実環境に近い形で検証
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/k1LoW/runn"
	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/diffeps"
	"github.com/ramsesyok/runnora/internal/evidence"
	"github.com/ramsesyok/runnora/internal/hook"
	"github.com/ramsesyok/runnora/internal/oracle"
	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/reporter"
	"github.com/ramsesyok/runnora/internal/scenario"
	"gopkg.in/yaml.v3"
)

// AppError は終了コードを持つアプリケーションエラー。
//
// main.go で errors.As(err, &appErr) により検査し、
// appErr.ExitCode を os.Exit に渡す。
//
// 終了コード体系:
//
//	0: 全成功
//	1: runbook 失敗 (アサーション失敗など)
//	2: 設定・引数不正 (runbook パス未指定、config ファイル読み込み失敗)
//	3: DB 接続失敗
//	4: before/after フック失敗
//	5: レポート出力失敗 (cmd/run.go で生成)
type AppError struct {
	ExitCode int
	Cause    error
}

// hookError はフック実行中に発生したエラーを示すラッパー。
//
// 設計の意図:
//
//	runn の BeforeFunc/AfterFunc でエラーが発生すると、
//	runn は runbook を失敗として記録する (RunResult.Err にエラーが入る)。
//	しかし runbook 失敗 (exit 1) とフック失敗 (exit 4) では終了コードが異なる。
//
//	そこでフックエラーを hookError でラップし、
//	op.Operators() の結果を処理する際に errors.As(rr.Err, &hErr) で
//	「これはフック起因の失敗か」を区別できるようにしている。
type hookError struct {
	cause error
}

func (h *hookError) Error() string { return h.cause.Error() }
func (h *hookError) Unwrap() error { return h.cause }

func (e *AppError) Error() string {
	return fmt.Sprintf("exit %d: %v", e.ExitCode, e.Cause)
}

func (e *AppError) Unwrap() error { return e.Cause }

// ExecutorFactory は OracleConfig から oracle.Executor を生成するファクトリ関数型。
//
// これが「テスト容易性のための DI ポイント」。
//
//	本番: DefaultExecutorFactory → oracle.Open → OracleExecutor
//	テスト: WithExecutorFactory(stub) → stub Executor (Oracle 不要)
//
// Runner に直接 *sql.DB を渡さずファクトリ関数を渡す設計にすることで、
// 接続エラーのシナリオ (エラーを返す factory) もテストできる。
type ExecutorFactory func(*config.OracleConfig) (oracle.Executor, error)

// DefaultExecutorFactory は oracle.Open を使って OracleExecutor を生成する。
// 本番環境で使用するデフォルトの factory。
func DefaultExecutorFactory(cfg *config.OracleConfig) (oracle.Executor, error) {
	db, err := oracle.Open(cfg)
	if err != nil {
		return nil, err
	}
	return oracle.NewOracleExecutor(db), nil
}

// Runner は runbook の実行を制御する。
//
// Runner のゼロ値は使用不可。必ず NewRunner() で生成すること。
type Runner struct {
	factory ExecutorFactory
	setenv  func(key, value string) error
}

// RunnerOption は Runner の設定オプション。Functional Options パターンを使用。
type RunnerOption func(*Runner)

// WithExecutorFactory は ExecutorFactory を差し替えるオプション。
//
// テスト時に stub executor を注入するために使う:
//
//	runner := app.NewRunner(app.WithExecutorFactory(func(cfg *config.OracleConfig) (oracle.Executor, error) {
//	    return &stubExecutor{}, nil  // Oracle 不要
//	}))
func WithExecutorFactory(f ExecutorFactory) RunnerOption {
	return func(r *Runner) { r.factory = f }
}

// WithSetenv は環境の変数をプロセスの環境変数に設定する関数を差し替えるオプション。
//
// runn は runbook の ${VAR} をプロセスの環境変数で展開するため、既定では os.Setenv を使う。
// テストでは t.Setenv を渡して、テスト終了時に元に戻す。
func WithSetenv(f func(key, value string) error) RunnerOption {
	return func(r *Runner) { r.setenv = f }
}

// NewRunner は Runner を生成する。
// デフォルトでは DefaultExecutorFactory (実 Oracle DB) を使用する。
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{factory: DefaultExecutorFactory, setenv: os.Setenv}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Plan は 1 回の run で実行する内容。cmd 層が runnora.yaml と引数から組み立てる。
type Plan struct {
	// Root は runnora: ブロックの SQL パスの基準 (プロジェクトルート、なければカレントディレクトリ)。
	Root string
	// ProjectName はレポートに載せる project.name。
	ProjectName string
	// Env は変数を展開済みの環境。環境を使わない場合は nil。
	Env *project.Resolved
	// Suite はレポートに載せるスイート名。
	Suite string
	// Runbooks は実行する runbook (実行順)。
	Runbooks []*scenario.Runbook
	// Scopes は runn に追加で許可するスコープ。
	Scopes []string
	// Trace は runn のトレースを有効にする。
	Trace bool
	// FailFast は最初の不合格で残りを実行しない。
	FailFast bool
	// EvidenceDir は証跡の保存先 (絶対パス)。"" なら保存しない。runbook ごとに <EvidenceDir>/<シナリオID>/ に書く。
	EvidenceDir string
	// EvidenceMode は証跡に書く範囲 (response / full)。
	EvidenceMode evidence.Mode
	// EvidenceMask は証跡で隠すもの。nil なら決まったヘッダだけを隠す。
	EvidenceMask *evidence.Mask
	// Warn は警告の出力先 (dump ステップの重複など)。nil なら出さない。
	Warn io.Writer
	// Version はレポートに載せる runnora のバージョン。
	Version string
}

// target は 1 つの runbook と、その前後処理のファイル。
type target struct {
	rb     *scenario.Runbook
	before []string
	after  []string
	// skip は環境が envs の対象外で実行しないことを表す。
	skip bool
}

// Run は Plan に従って runbook を実行し、Report を返す。
//
// 処理の流れ:
//  1. 引数バリデーション (runbook なし → exit 2)
//  2. 環境の変数をプロセスの環境変数に設定する (runn が runbook の ${VAR} を展開するため)
//  3. runbook ごとに前後処理のファイルを組み立てる (envs の対象外は実行しない)
//  4. Resolver で全 SQL ファイルの存在を確認する (失敗 → exit 4)
//  5. 前後処理があるときだけ Oracle に接続する (未設定 → exit 2、接続失敗 → exit 3)
//  6. runbook を 1 つずつ runn で実行し、実際の結果を expect と比べる
//
// 返り値:
//   - (*reporter.Report, nil): すべて期待どおり
//   - (*reporter.Report, *AppError{ExitCode:4}): 期待どおりでなく、実際はフック失敗のものがある
//   - (*reporter.Report, *AppError{ExitCode:1}): それ以外で期待どおりでないものがある
//   - (nil, *AppError{ExitCode:2,3,4}): 実行前の初期化失敗
func (r *Runner) Run(ctx context.Context, plan *Plan) (*reporter.Report, error) {
	if plan == nil || len(plan.Runbooks) == 0 {
		return nil, &AppError{ExitCode: 2, Cause: fmt.Errorf("at least one runbook path is required")}
	}

	envName := ""
	var envBefore, envAfter []string
	if plan.Env != nil {
		envName = plan.Env.Name
		envBefore, envAfter = plan.Env.Before, plan.Env.After
		for _, k := range sortedKeys(plan.Env.Vars) {
			if err := r.setenv(k, plan.Env.Vars[k]); err != nil {
				return nil, &AppError{ExitCode: 2, Cause: fmt.Errorf("set %s: %w", k, err)}
			}
		}
	}

	started := time.Now()
	report := newReport(plan, started)
	defer func() { report.ElapsedMs = time.Since(started).Milliseconds() }()
	var targets []target
	var allFiles []string
	for _, rb := range plan.Runbooks {
		if !rb.AllowedIn(envName) {
			targets = append(targets, target{rb: rb, skip: true})
			continue
		}
		scBefore, scAfter := rb.SQLFiles(plan.Root)
		before, after := hook.Order(envBefore, envAfter, scBefore, scAfter)
		targets = append(targets, target{rb: rb, before: before, after: after})
		allFiles = append(append(allFiles, before...), after...)
	}

	// フックファイルの存在確認: 実行前に全ての欠損を検出して報告
	if err := hook.NewResolver().Validate(allFiles); err != nil {
		return nil, &AppError{ExitCode: 4, Cause: err}
	}

	var exec oracle.Executor
	if len(allFiles) > 0 {
		// 前後処理がある場合だけ Oracle に接続する。DB を使わない runbook だけなら接続しない。
		if plan.Env == nil || !plan.Env.HasOracle {
			return nil, &AppError{ExitCode: 2, Cause: fmt.Errorf("前後処理の SQL がありますが、環境に oracle が設定されていません (runnora.yaml の environments.<名前>.oracle)")}
		}
		var err error
		exec, err = r.factory(&plan.Env.Oracle)
		if err != nil {
			return nil, &AppError{ExitCode: 3, Cause: fmt.Errorf("oracle: %w", err)}
		}
		defer exec.Close() // 関数終了時に接続プールを確実に閉じる
	}

	base := baseRunnOptions(plan)
	warnDump := dumpWarner(plan.Warn, plan.Root)
	for _, t := range targets {
		path := displayPath(plan.Root, t.rb.Path)
		expect := t.rb.Expect()
		if t.skip {
			// envs の対象外は実行せず、レポートに skipped として載せる (実行順を保つ)
			report.Skipped++
			report.Results = append(report.Results, reporter.RunResult{
				ID: t.rb.ID, Path: path, Expect: expect, Actual: scenario.OutcomeSkipped, Passed: true,
			})
			continue
		}
		warnDump(t.rb)
		// 証跡を保存しない場合も、diffEps の記録のために実行中のステップだけを追う
		mode := evidence.ModeOff
		scenarioDir := ""
		if plan.EvidenceDir != "" {
			mode = plan.EvidenceMode
			scenarioDir = filepath.Join(plan.EvidenceDir, evidence.SafeName(t.rb.ID))
			// 旧来の dump ステップ ({{ env.RUNNORA_EVIDENCE_DIR }}/...) も同じフォルダに書かれるようにする
			if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
				return nil, &AppError{ExitCode: 5, Cause: fmt.Errorf("evidence: %w", err)}
			}
			if err := r.setenv(EvidenceDirEnv, scenarioDir); err != nil {
				return nil, &AppError{ExitCode: 2, Cause: fmt.Errorf("set %s: %w", EvidenceDirEnv, err)}
			}
		}
		capt := evidence.New(t.rb.ID, mode, plan.EvidenceMask)
		diffs := diffeps.NewRecorder(plan.Root, capt.CurrentStep)
		opts := append(append([]runn.Option{}, base...), runn.Capture(capt), runn.Func(diffeps.FuncName, diffs.Func()))
		out := r.runOne(ctx, t, exec, opts, plan.Root)
		passed := out.actual == expect
		res := reporter.RunResult{
			ID: t.rb.ID, Desc: t.rb.Desc, Path: path, Expect: expect, Actual: out.actual, Passed: passed, Error: out.msg,
			ElapsedMs: out.elapsed.Milliseconds(), Hooks: out.hooks, Steps: out.steps,
		}
		if scenarioDir != "" {
			written, err := capt.Flush(scenarioDir, plan.EvidenceDir)
			if err != nil && plan.Warn != nil {
				fmt.Fprintf(plan.Warn, "警告: %s の証跡の一部を保存できませんでした: %v\n", t.rb.ID, err)
			}
			attachEvidence(res.Steps, written)
		}
		if err := attachDiffs(res.Steps, diffs.Calls(), scenarioDir, plan.EvidenceDir); err != nil && plan.Warn != nil {
			fmt.Fprintf(plan.Warn, "警告: %s の diffEps の差分を保存できませんでした: %v\n", t.rb.ID, err)
		}
		if scenarioDir != "" {
			_ = os.Remove(scenarioDir) // 何も書かなかった場合だけ消える (中身があれば失敗する)
		}
		report.Results = append(report.Results, res)
		if passed {
			report.Passed++
		} else {
			report.Failed++
		}
		// --fail-fast が有効なら最初の不合格で残りの runbook をスキップする
		if plan.FailFast && !passed {
			break
		}
	}
	report.Total = report.Passed + report.Failed + report.Skipped

	hookFailed, runFailed := 0, 0
	for _, res := range report.Results {
		if res.Passed {
			continue
		}
		if res.Actual == scenario.OutcomeHookFail {
			hookFailed++
		} else {
			runFailed++
		}
	}
	// 実際の結果がフック失敗のものは runbook 失敗より優先して報告する (exit 4 > exit 1)
	if hookFailed > 0 {
		return report, &AppError{ExitCode: 4, Cause: fmt.Errorf("%d hook(s) failed", hookFailed)}
	}
	if runFailed > 0 {
		return report, &AppError{ExitCode: 1, Cause: fmt.Errorf("%d runbook(s) did not meet expectations", runFailed)}
	}
	return report, nil
}

// outcome は 1 つの runbook を実行した結果。
type outcome struct {
	// actual は実際の結果 (pass / fail / hookFail)。msg はエラー文。
	actual  string
	msg     string
	elapsed time.Duration
	hooks   []reporter.HookResult
	steps   []reporter.StepResult
}

// runOne は 1 つの runbook を実行し、実際の結果・前後処理・ステップの結果を返す。
func (r *Runner) runOne(ctx context.Context, t target, exec oracle.Executor, base []runn.Option, root string) (out outcome) {
	onFile := func(phase, file string, err error) {
		h := reporter.HookResult{Phase: phase, File: displayPath(root, file), OK: err == nil}
		if err != nil {
			h.Error = err.Error()
		}
		out.hooks = append(out.hooks, h)
	}
	opts := append([]runn.Option{}, base...)
	if len(t.before) > 0 {
		// BeforeFunc は runn が runbook を実行する直前に呼ばれる (include 先では呼ばれない)。
		opts = append(opts, runn.BeforeFunc(func(*runn.RunResult) error {
			if err := hook.RunBeforeWith(ctx, exec, t.before, onFile); err != nil {
				// hookError でラップして runbook 失敗と区別できるようにする
				return &hookError{cause: err}
			}
			return nil
		}))
	}
	if len(t.after) > 0 {
		// AfterFunc は runbook が失敗していても呼ばれる (cleanup guaranteed)。
		opts = append(opts, runn.AfterFunc(func(*runn.RunResult) error {
			if err := hook.RunAfterWith(ctx, exec, t.after, onFile); err != nil {
				return &hookError{cause: err}
			}
			return nil
		}))
	}

	start := time.Now()
	// out は名前付きの戻り値なので、return の後でも所要時間を設定できる
	defer func() { out.elapsed = time.Since(start) }()

	// runn.Load は runbook ファイル (YAML) を読み込んで Operator を生成する。
	op, err := runn.Load(t.rb.Path, opts...)
	if err != nil {
		// Load 失敗: ファイル形式不正など。runbook 失敗として扱う。
		out.actual, out.msg = scenario.OutcomeFail, err.Error()
		return out
	}
	// RunN はロードした Operator を実行する。エラーは RunResult.Err に格納される。
	op.RunN(ctx) //nolint:errcheck

	out.actual = scenario.OutcomePass
	keys := topLevelStepKeys(t.rb.Text)
	for _, o := range op.Operators() {
		rr := o.Result()
		out.steps = append(out.steps, flattenSteps(rr.StepResults, "", 0, keys)...)
		if rr.Err == nil {
			continue
		}
		out.msg = rr.Err.Error()
		// BeforeFunc/AfterFunc で返した hookError は runn が RunResult.Err にそのままセットする。
		var hErr *hookError
		if errors.As(rr.Err, &hErr) {
			out.actual = scenario.OutcomeHookFail
			continue
		}
		if out.actual != scenario.OutcomeHookFail {
			out.actual = scenario.OutcomeFail
		}
	}
	return out
}

// flattenSteps は runn のステップの結果を、include 先を含めて実行順に平らに並べる。
//
//   - prefix は include 先のステップのキーの前に付ける呼び出し元のキー ("inc." など)。
//   - index は include 先のステップに付けるトップレベルのステップの番号 (トップレベルでは 0)。
//   - keys はトップレベルのステップのキー (runbook の記述順)。runn が結果を持たない (nil) ステップの
//     キーをここから補う。
//   - loop で include した場合、runn は回ごとの結果を持たない (最後の回だけ) ので、ステップは 1 つになる。
//     証跡は回ごとに分かれる (attachEvidence が同じステップに付ける)。
func flattenSteps(results []*runn.StepResult, prefix string, index int, keys []string) []reporter.StepResult {
	var out []reporter.StepResult
	// runn は失敗したステップの後のステップを「飛ばした」として返すので、失敗の後は実行しなかったとする
	failed := false
	for i, sr := range results {
		if sr == nil {
			if prefix == "" && i < len(keys) {
				out = append(out, reporter.StepResult{Key: keys[i], Index: i + 1, Result: reporter.StepNotRun})
			}
			continue
		}
		key := sr.Key
		if key == "" {
			key = strconv.Itoa(sr.Index)
		}
		key = prefix + key
		idx := index
		if prefix == "" {
			idx = sr.Index + 1
		}
		s := reporter.StepResult{
			Key: key, Index: idx, Desc: sr.Desc, Runner: string(sr.RunnerType),
			Result: reporter.StepSuccess, ElapsedMs: sr.Elapsed.Milliseconds(),
		}
		switch {
		case sr.Skipped && failed:
			s.Result = reporter.StepNotRun
		case sr.Skipped:
			s.Result = reporter.StepSkipped
		case sr.Err != nil:
			s.Result = reporter.StepFailure
			s.Error = sr.Err.Error()
			failed = true
		}
		out = append(out, s)
		for j, inc := range sr.IncludedRunResults {
			if inc == nil {
				continue
			}
			p := key
			if len(sr.IncludedRunResults) > 1 {
				// loop で include した場合は回ごとに [n] を付ける (証跡のキーと同じ)
				p = fmt.Sprintf("%s[%d]", key, j)
			}
			out = append(out, flattenSteps(inc.StepResults, p+".", idx, nil)...)
		}
	}
	return out
}

// topLevelStepKeys は runbook の steps のキーを記述順に返す (steps が配列なら添字)。
func topLevelStepKeys(text string) []string {
	var doc struct {
		Steps yaml.Node `yaml:"steps"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil
	}
	var keys []string
	switch doc.Steps.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(doc.Steps.Content); i += 2 {
			keys = append(keys, doc.Steps.Content[i].Value)
		}
	case yaml.SequenceNode:
		for i := range doc.Steps.Content {
			keys = append(keys, strconv.Itoa(i))
		}
	}
	return keys
}

// loopIndex は証跡のキーの中の loop の回 ("[2]") に一致する。
var loopIndex = regexp.MustCompile(`\[\d+\]`)

// attachEvidence は証跡ファイルを、同じキーのステップに付ける。
// runn のステップの結果は loop の回をまとめて 1 つなので、見つからなければ [n] を除いたキーで探す。
func attachEvidence(steps []reporter.StepResult, written []evidence.Written) {
	byKey := map[string]int{}
	for i, s := range steps {
		if _, ok := byKey[s.Key]; !ok {
			byKey[s.Key] = i
		}
	}
	for _, w := range written {
		i, ok := byKey[w.Key]
		if !ok {
			i, ok = byKey[loopIndex.ReplaceAllString(w.Key, "")]
		}
		if ok {
			steps[i].Evidence = append(steps[i].Evidence, w.Path)
		}
	}
}

// EvidenceDirEnv は旧来の dump ステップが証跡の保存先として参照する環境変数。
// 証跡を自動保存するときは、runbook ごとにそのシナリオの証跡フォルダを設定する。
const EvidenceDirEnv = "RUNNORA_EVIDENCE_DIR"

// dumpStepPattern は runbook の dump ステップ (steps の中の "dump:" キー) に一致する。
var dumpStepPattern = regexp.MustCompile(`(?m)^\s+dump:\s*(#.*)?$`)

// HasDumpStep は runbook の文字列が dump ステップを含むかを返す (コメント行は除く)。
func HasDumpStep(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if dumpStepPattern.MatchString(line) {
			return true
		}
	}
	return false
}

// dumpWarner は、dump ステップを持つ runbook (include 先を含む) ごとに 1 回だけ警告する関数を返す。
func dumpWarner(w io.Writer, root string) func(*scenario.Runbook) {
	if w == nil {
		return func(*scenario.Runbook) {}
	}
	warned := map[string]bool{}
	var visit func(path, text string)
	visit = func(path, text string) {
		if warned[path] {
			return
		}
		warned[path] = true
		if HasDumpStep(text) {
			fmt.Fprintf(w, "警告: %s: dump ステップは証跡の自動保存と重複しています (runnora-migrate で削除できます)\n", displayPath(root, path))
		}
	}
	return func(rb *scenario.Runbook) {
		visit(rb.Path, rb.Text)
		for _, inc := range scenario.Includes(rb) {
			if warned[inc] {
				continue
			}
			b, err := os.ReadFile(inc)
			if err != nil {
				continue
			}
			visit(inc, string(b))
		}
	}
}

// baseRunnOptions はすべての runbook に共通の runn オプションを返す。
//
//   - runn.Scopes("read:parent"): runbook から親ディレクトリのファイルを読むことを許可
//   - runn.Profile:               ステップの所要時間を測る (report.json の elapsedMs)
//   - runn.Trace:                 トレース出力 (--trace / runn.trace)
//   - runn.FailFast:              runbook 内で最初の失敗で停止
func baseRunnOptions(plan *Plan) []runn.Option {
	opts := []runn.Option{
		runn.Scopes(append([]string{"read:parent"}, plan.Scopes...)...),
		// runn はプロファイル (ストップウォッチ) を有効にしたときだけ、ステップの所要時間を結果に入れる
		runn.Profile(true),
	}
	if plan.Trace {
		opts = append(opts, runn.Trace(true))
	}
	if plan.FailFast {
		opts = append(opts, runn.FailFast(true))
	}
	return opts
}

func newReport(plan *Plan, started time.Time) *reporter.Report {
	rep := &reporter.Report{
		SchemaVersion: reporter.SchemaVersion, Runnora: plan.Version, StartedAt: started.Format(time.RFC3339),
		Project: plan.ProjectName, Suite: plan.Suite,
	}
	if plan.Env != nil && plan.Env.Name != "" {
		rep.Env = &reporter.EnvInfo{Name: plan.Env.Name, Description: plan.Env.Description, Overrides: plan.Env.Overrides}
		if len(plan.Env.Backends) > 0 {
			rep.Backends = map[string]reporter.Backend{}
			for k, b := range plan.Env.Backends {
				rep.Backends[k] = reporter.Backend{Mode: b.Mode, Note: b.Note}
			}
		}
	}
	return rep
}

// displayPath はレポートに載せる runbook のパスを返す。
// 基準ディレクトリの中にあれば相対パス (/ 区切り)、外にあれば絶対パスのまま。
func displayPath(root, path string) string {
	if root == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(rel)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// attachDiffs は diffEps の呼び出しのうち差分があったものを、証跡に .diff.json として書き
// (scenarioDir が空なら書かない)、失敗したステップの Diffs に要約を付ける。
//
// ファイル名は <番号>-<キー>.diff.json。同じステップで 2 回目以降は .diff.<n>.json。
func attachDiffs(steps []reporter.StepResult, calls []diffeps.Call, scenarioDir, evidenceDir string) error {
	byKey := map[string]int{}
	for i, s := range steps {
		if _, ok := byKey[s.Key]; !ok {
			byKey[s.Key] = i
		}
	}
	var errs []error
	used := map[string]int{}
	for _, c := range calls {
		if c.Result == nil || c.Result.Equal {
			continue
		}
		sum := reporter.DiffSummary{Differences: c.Result.Summary.Differences}
		for i, d := range c.Result.Differences {
			if i == reporter.MaxDiffItems {
				break
			}
			sum.Items = append(sum.Items, d)
		}
		if scenarioDir != "" {
			name := fmt.Sprintf("%02d-%s.diff", c.Index, evidence.SafeName(c.Key))
			used[name]++
			if n := used[name]; n > 1 {
				name = fmt.Sprintf("%s.%d", name, n)
			}
			path := filepath.Join(scenarioDir, name+".json")
			b, err := json.MarshalIndent(c.Result, "", "  ")
			if err == nil {
				err = os.MkdirAll(scenarioDir, 0o755)
			}
			if err == nil {
				err = os.WriteFile(path, append(b, '\n'), 0o644)
			}
			if err != nil {
				errs = append(errs, err)
			} else if rel, err := filepath.Rel(evidenceDir, path); err == nil {
				sum.File = filepath.ToSlash(rel)
			}
		}
		i, ok := byKey[c.Key]
		if !ok {
			i, ok = byKey[loopIndex.ReplaceAllString(c.Key, "")]
		}
		// 差分があっても、!diffEps(...) のように差分を期待したステップは成功しているので要約を付けない
		if ok && steps[i].Result == reporter.StepFailure {
			steps[i].Diffs = append(steps[i].Diffs, sum)
		}
	}
	return errors.Join(errs...)
}
