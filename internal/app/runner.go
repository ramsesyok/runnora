// Package app は runnora のコアロジックを提供する。
//
// 主な責務:
//   - config の読み込みと Oracle DB 接続の確立
//   - before/after フックの実行 (SQL/PL/SQL ファイル)
//   - runn を使った runbook の実行と結果の集計
//   - 終了コードを持つエラー (AppError) の生成
//
// 依存関係:
//
//	app.Runner
//	  ├── config.Load (設定読み込み)
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
	"errors"
	"fmt"

	"github.com/k1LoW/runn"
	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/hook"
	"github.com/ramsesyok/runnora/internal/oracle"
	"github.com/ramsesyok/runnora/internal/reporter"
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

// NewRunner は Runner を生成する。
// デフォルトでは DefaultExecutorFactory (実 Oracle DB) を使用する。
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{factory: DefaultExecutorFactory}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Run は設定とオプションに従って runbook を実行し、Report を返す。
//
// 処理の流れ:
//  1. 引数バリデーション (runbook パス未指定 → exit 2)
//  2. config.Load で設定ファイルを読み込む (失敗 → exit 2)
//  3. ExecutorFactory で Oracle DB に接続する (失敗 → exit 3)
//  4. BuildBeforeFiles / BuildAfterFiles でフックファイルリストを組み立てる
//  5. Resolver.Validate でフックファイルの存在を確認する (失敗 → exit 4)
//  6. buildRunnOptions で runn のオプションを構築する
//  7. 各 runbook パスに対して runn.Load → op.RunN を実行する
//  8. 結果を集計し Report を返す
//
// 返り値:
//   - (*reporter.Report, nil): 全成功
//   - (*reporter.Report, *AppError{ExitCode:1}): runbook 失敗
//   - (*reporter.Report, *AppError{ExitCode:4}): フック失敗
//   - (nil, *AppError{ExitCode:2,3,4}): 実行前の初期化失敗
func (r *Runner) Run(ctx context.Context, opts *config.RunOptions) (*reporter.Report, error) {
	// 引数バリデーション: runbook パスが 1 つ以上必要
	if len(opts.RunbookPaths) == 0 {
		return nil, &AppError{ExitCode: 2, Cause: fmt.Errorf("at least one runbook path is required")}
	}

	// 設定ロード: YAML ファイルを読み込んでデフォルト値を適用
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, &AppError{ExitCode: 2, Cause: err}
	}

	// フックファイルリストを組み立てる。
	// 順序: config.common.before → CLI --before-sql (before)
	//        CLI --after-sql → config.common.after (after)
	beforeFiles := config.BuildBeforeFiles(cfg, opts)
	afterFiles := config.BuildAfterFiles(cfg, opts)
	needsDB := len(beforeFiles) > 0 || len(afterFiles) > 0

	var exec oracle.Executor
	if needsDB {
		// Oracle 接続: SQL フックが設定されている場合のみ ExecutorFactory 経由で Executor を取得する。
		// DB を使わない runbook では oracle.dsn が空でも実行できる。
		var err error
		exec, err = r.factory(&cfg.Oracle)
		if err != nil {
			return nil, &AppError{ExitCode: 3, Cause: fmt.Errorf("oracle: %w", err)}
		}
		defer exec.Close() // 関数終了時に接続プールを確実に閉じる
	}

	// フックファイルの存在確認: 実行前に全ての欠損を検出して報告
	resolver := hook.NewResolver()
	if err := resolver.Validate(beforeFiles); err != nil {
		return nil, &AppError{ExitCode: 4, Cause: err}
	}
	if err := resolver.Validate(afterFiles); err != nil {
		return nil, &AppError{ExitCode: 4, Cause: err}
	}

	// runn オプションを構築する (before/after フックを含む)
	runnOpts := buildRunnOptions(ctx, cfg, opts, exec, beforeFiles, afterFiles)

	// 複数 runbook を順に実行し、結果を集計する
	total, passed, hookFailed, runFailed := 0, 0, 0, 0
	var results []reporter.RunResult

	for _, path := range opts.RunbookPaths {
		// runn.Load は runbook ファイル (YAML) を読み込んで Operator を生成する。
		// glob パターンや複数ファイルも扱えるが、ここでは 1 パスずつ処理する。
		op, err := runn.Load(path, runnOpts...)
		if err != nil {
			// Load 失敗: ファイル形式不正など。runbook 実行失敗と同等に扱う。
			runFailed++
			total++
			results = append(results, reporter.RunResult{
				Path:   path,
				Passed: false,
				Error:  err.Error(),
			})
			continue
		}

		// RunN はロードした全 Operator を順に実行する。
		// エラーは RunResult.Err に格納されるため、戻り値は無視してよい。
		op.RunN(ctx) //nolint:errcheck

		// 各 Operator (= 各 runbook) の結果を収集する
		for _, o := range op.Operators() {
			rr := o.Result()
			total++
			if rr.Err != nil {
				// hookError か否かで終了コードを分岐する。
				// BeforeFunc/AfterFunc で返した hookError は
				// runn が RunResult.Err にそのままセットする。
				var hErr *hookError
				if errors.As(rr.Err, &hErr) {
					hookFailed++ // フック失敗 → exit 4
				} else {
					runFailed++ // runbook 失敗 → exit 1
				}
				results = append(results, reporter.RunResult{
					Path:   rr.Path,
					Passed: false,
					Error:  rr.Err.Error(),
				})
			} else {
				passed++
				results = append(results, reporter.RunResult{
					Path:   rr.Path,
					Passed: true,
				})
			}
		}

		// --fail-fast が有効なら最初の失敗で残りの runbook をスキップする
		if opts.FailFast && (hookFailed+runFailed) > 0 {
			break
		}
	}

	report := &reporter.Report{
		Total:   total,
		Passed:  passed,
		Failed:  hookFailed + runFailed,
		Results: results,
	}

	// フック失敗は runbook 失敗より優先して報告する (exit 4 > exit 1)
	if hookFailed > 0 {
		return report, &AppError{ExitCode: 4, Cause: fmt.Errorf("%d hook(s) failed", hookFailed)}
	}
	if runFailed > 0 {
		return report, &AppError{ExitCode: 1, Cause: fmt.Errorf("%d runbook(s) failed", runFailed)}
	}

	return report, nil
}

// buildRunnOptions は runn.Load に渡すオプションを構築する。
//
// 主なオプション:
//   - runn.Scopes("read:parent"): runbook から親ディレクトリのファイルを読むことを許可
//     (runn はデフォルトでサンドボックス動作するため明示的に許可が必要)
//   - runn.BeforeFunc: 各 runbook の実行前に before フックを実行する
//   - runn.AfterFunc:  各 runbook の実行後に after フックを実行する
//   - runn.FailFast:   --fail-fast フラグが有効な場合に追加
//
// hookError によるエラーのラップ:
//
//	フックエラーを hookError でラップして返すことで、
//	Runner.Run 側で errors.As(rr.Err, &hErr) によって
//	フック起因の失敗かを識別できる。
func buildRunnOptions(
	ctx context.Context,
	cfg *config.Config,
	opts *config.RunOptions,
	exec oracle.Executor,
	beforeFiles, afterFiles []string,
) []runn.Option {
	runnOpts := []runn.Option{
		// runbook ファイルから親ディレクトリ (../) へのアクセスを許可する。
		// 例: runbook が ./runbooks/ にあり、SQL ファイルが ./sql/ にある構成で必要。
		runn.Scopes("read:parent"),
	}

	if len(beforeFiles) > 0 {
		// BeforeFunc は runn が各 runbook を実行する直前に呼ばれる。
		// before SQL ファイルをここで実行することで、テスト用 DB 状態を整備する。
		runnOpts = append(runnOpts, runn.BeforeFunc(func(rr *runn.RunResult) error {
			if err := hook.RunBefore(ctx, exec, beforeFiles); err != nil {
				// hookError でラップして Runner.Run 側で識別できるようにする
				return &hookError{cause: err}
			}
			return nil
		}))
	}

	if len(afterFiles) > 0 {
		// AfterFunc は runn が各 runbook を実行した直後に呼ばれる。
		// after SQL ファイルをここで実行して DB を後始末する。
		// runbook が失敗していても AfterFunc は呼ばれる (cleanup guaranteed)。
		runnOpts = append(runnOpts, runn.AfterFunc(func(rr *runn.RunResult) error {
			if err := hook.RunAfter(ctx, exec, afterFiles); err != nil {
				return &hookError{cause: err}
			}
			return nil
		}))
	}

	if opts.FailFast {
		// FailFast(true) を指定すると、最初の runbook 失敗で残りの実行を中断する。
		runnOpts = append(runnOpts, runn.FailFast(true))
	}

	// cfg は現在は使用していないが、将来的に DB ランナーの登録
	// (runn.DBRunner(cfg.Runn.DBRunnerName, db)) などで使用する予定。
	_ = cfg

	return runnOpts
}
