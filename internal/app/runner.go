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
type AppError struct {
	ExitCode int
	Cause    error
}

// hookError はフック実行中に発生したエラーを示すラッパー。
type hookError struct {
	cause error
}

func (h *hookError) Error() string { return h.cause.Error() }
func (h *hookError) Unwrap() error { return h.cause }

func (e *AppError) Error() string {
	return fmt.Sprintf("exit %d: %v", e.ExitCode, e.Cause)
}

func (e *AppError) Unwrap() error { return e.Cause }

// ExecutorFactory は DBConfig から oracle.Executor を生成するファクトリ関数型。
type ExecutorFactory func(*config.DBConfig) (oracle.Executor, error)

// DefaultExecutorFactory は oracle.Open を使って OracleExecutor を生成する。
func DefaultExecutorFactory(cfg *config.DBConfig) (oracle.Executor, error) {
	db, err := oracle.Open(cfg)
	if err != nil {
		return nil, err
	}
	return oracle.NewOracleExecutor(db), nil
}

// Runner は runbook の実行を制御する。
type Runner struct {
	factory ExecutorFactory
}

// RunnerOption は Runner の設定オプション。
type RunnerOption func(*Runner)

// WithExecutorFactory は ExecutorFactory を差し替えるオプション（テスト用）。
func WithExecutorFactory(f ExecutorFactory) RunnerOption {
	return func(r *Runner) { r.factory = f }
}

// NewRunner は Runner を生成する。
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{factory: DefaultExecutorFactory}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Run は設定とオプションに従って runbook を実行し、Report を返す。
func (r *Runner) Run(ctx context.Context, opts *config.RunOptions) (*reporter.Report, error) {
	// 引数バリデーション
	if len(opts.RunbookPaths) == 0 {
		return nil, &AppError{ExitCode: 2, Cause: fmt.Errorf("at least one runbook path is required")}
	}

	// 設定ロード
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, &AppError{ExitCode: 2, Cause: err}
	}

	// Oracle 接続
	exec, err := r.factory(&cfg.DB)
	if err != nil {
		return nil, &AppError{ExitCode: 3, Cause: fmt.Errorf("db: %w", err)}
	}
	defer exec.Close()

	// before/after ファイルリストを組み立てる
	beforeFiles := config.BuildBeforeFiles(cfg, opts)
	afterFiles := config.BuildAfterFiles(cfg, opts)

	// フックファイルの存在確認
	resolver := hook.NewResolver()
	if err := resolver.Validate(beforeFiles); err != nil {
		return nil, &AppError{ExitCode: 4, Cause: err}
	}
	if err := resolver.Validate(afterFiles); err != nil {
		return nil, &AppError{ExitCode: 4, Cause: err}
	}

	// runn オプションを構築する
	runnOpts := buildRunnOptions(ctx, cfg, opts, exec, beforeFiles, afterFiles)

	// 複数 runbook を順に実行する
	total, passed, hookFailed, runFailed := 0, 0, 0, 0
	var results []reporter.RunResult

	for _, path := range opts.RunbookPaths {
		op, err := runn.Load(path, runnOpts...)
		if err != nil {
			runFailed++
			total++
			results = append(results, reporter.RunResult{
				Path:   path,
				Passed: false,
				Error:  err.Error(),
			})
			continue
		}

		op.RunN(ctx) //nolint:errcheck

		for _, o := range op.Operators() {
			rr := o.Result()
			total++
			if rr.Err != nil {
				var hErr *hookError
				if errors.As(rr.Err, &hErr) {
					hookFailed++
				} else {
					runFailed++
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

	if hookFailed > 0 {
		return report, &AppError{ExitCode: 4, Cause: fmt.Errorf("%d hook(s) failed", hookFailed)}
	}
	if runFailed > 0 {
		return report, &AppError{ExitCode: 1, Cause: fmt.Errorf("%d runbook(s) failed", runFailed)}
	}

	return report, nil
}

func buildRunnOptions(
	ctx context.Context,
	cfg *config.Config,
	opts *config.RunOptions,
	exec oracle.Executor,
	beforeFiles, afterFiles []string,
) []runn.Option {
	runnOpts := []runn.Option{
		runn.Scopes("read:parent"),
		runn.BeforeFunc(func(rr *runn.RunResult) error {
			if err := hook.RunBefore(ctx, exec, beforeFiles); err != nil {
				return &hookError{cause: err}
			}
			return nil
		}),
		runn.AfterFunc(func(rr *runn.RunResult) error {
			if err := hook.RunAfter(ctx, exec, afterFiles); err != nil {
				return &hookError{cause: err}
			}
			return nil
		}),
	}

	if opts.FailFast {
		runnOpts = append(runnOpts, runn.FailFast(true))
	}

	_ = cfg // 将来 DBRunner 登録に使用

	return runnOpts
}
