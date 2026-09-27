package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/reporter"
	"github.com/ramsesyok/runnora/internal/scenario"
)

// newRunCmd は "runnora run" サブコマンドを生成する。
//
// 処理の流れ:
//  1. runnora.yaml を探して読み込み、環境を選んで変数を展開する
//  2. --suite ならスイートの条件で、そうでなければ引数で実行する runbook を決める
//  3. app.Runner.Run で前後処理 → runn 実行 → 期待する結果との照合
//  4. reporter でレポートを出力 (stdout またはファイル)
//  5. エラーを返すと main が ExitCode を解決して os.Exit を呼ぶ
func newRunCmd() *cobra.Command {
	var (
		projectPath  string
		envName      string
		suiteName    string
		vars         []string
		reportFormat string
		reportOut    string
		trace        bool
		failFast     bool
		scopes       []string
	)

	cmd := &cobra.Command{
		Use:   "run [options] [runbook...]",
		Short: "runbook を実行する",
		Long: `runbook を実行する。

実行する runbook は、引数で指定するか、--suite で runnora.yaml のスイートを指定する。
接続先などの変数と共通の前後処理は runnora.yaml の環境 (--env) から、
シナリオ固有の前後処理と期待する結果は各 runbook の runnora: ブロックから読む。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 引数の解析が済んだ後のエラーでは使い方を表示しない (エラーが読みにくくなるため)
			cmd.SilenceUsage = true
			if err := rejectLegacyFlags(cmd); err != nil {
				return err
			}
			if suiteName != "" && len(args) > 0 {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("--suite と runbook の引数は同時に指定できません")}
			}
			if suiteName == "" && len(args) == 0 {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("at least one runbook path is required (runbook を指定するか --suite を指定してください)")}
			}
			cliVars, err := parseVars(vars)
			if err != nil {
				return &app.AppError{ExitCode: 2, Cause: err}
			}

			lp, err := loadProject(projectPath, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			plan, err := buildRunPlan(lp, envName, suiteName, cliVars, args)
			if err != nil {
				return err
			}
			if plan.Env != nil && len(plan.Env.Overrides) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "env override: %s\n", strings.Join(plan.Env.Overrides, ", "))
			}

			// runnora.yaml を基準に、明示された CLI フラグを優先する。
			if lp.P != nil {
				if !cmd.Flags().Changed("report-format") && lp.P.Report.Format != "" {
					reportFormat = lp.P.Report.Format
				}
				if !cmd.Flags().Changed("report-out") && plan.Env != nil && plan.Env.ReportOutput != "" {
					reportOut = lp.P.Abs(plan.Env.ReportOutput)
				}
				scopes = append(append([]string{}, lp.P.Runn.Scopes...), scopes...)
				trace = trace || lp.P.Runn.Trace
			}
			plan.Scopes = scopes
			plan.Trace = trace
			plan.FailFast = failFast

			// 不正な形式では runbook やフックを実行しない。
			stdoutReporter, rerr := reporter.NewReporter(reportFormat, cmd.OutOrStdout())
			if rerr != nil {
				return &app.AppError{ExitCode: 2, Cause: rerr}
			}

			runner := app.NewRunner()
			report, err := runner.Run(cmd.Context(), plan)

			// report != nil なら runbook は少なくとも一部実行されている。
			// エラーがあっても先にレポートを出力し、その後エラーを返す。
			if report != nil {
				rep := stdoutReporter
				if reportOut != "" {
					r, rerr := reporter.NewFileReporter(reportFormat, reportOut)
					if rerr != nil {
						return &app.AppError{ExitCode: 5, Cause: fmt.Errorf("report: %w", rerr)}
					}
					rep = r
				}
				writeErr := rep.Write(report)
				if reportOut != "" {
					if closeErr := rep.Close(); writeErr == nil {
						writeErr = closeErr
					}
				}
				if writeErr != nil {
					return &app.AppError{ExitCode: 5, Cause: fmt.Errorf("report write: %w", writeErr)}
				}
			}

			return err
		},
	}

	// --- フラグ定義 ---
	cmd.Flags().StringVar(&projectPath, "project", "", "runnora.yaml のパス (省略時はカレントディレクトリから親へ探す)")
	cmd.Flags().StringVar(&envName, "env", "", "使う環境 (runnora.yaml の environments の名前)")
	cmd.Flags().StringVar(&suiteName, "suite", "", "実行するスイート (runnora.yaml の suites の名前)")
	cmd.Flags().StringArrayVar(&vars, "var", nil, "変数の上書き NAME=VALUE (複数指定可。OS の環境変数と runnora.yaml より優先)")
	cmd.Flags().StringVar(&reportFormat, "report-format", "text", "レポート形式 (text|json|junit)")
	cmd.Flags().StringVar(&reportOut, "report-out", "", "レポート出力先ファイル（省略時は標準出力）")
	cmd.Flags().BoolVar(&trace, "trace", false, "トレースモードを有効にする")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "最初の失敗で停止する")
	cmd.Flags().StringSliceVar(&scopes, "scopes", nil, "runn に追加で許可するスコープ（例: run:exec）")
	addLegacyFlags(cmd, "config", "before-sql", "after-sql")

	return cmd
}

// buildRunPlan は runnora.yaml と引数から実行内容を組み立てる。
func buildRunPlan(lp *loadedProject, envFlag, suiteName string, cliVars map[string]string, args []string) (*app.Plan, error) {
	plan := &app.Plan{Root: lp.Root, Suite: suiteName}
	if lp.P == nil {
		if envFlag != "" || suiteName != "" {
			return nil, &app.AppError{ExitCode: 2, Cause: fmt.Errorf("--env / --suite を使うには runnora.yaml が必要です (runnora init で作成できます)")}
		}
		if len(cliVars) > 0 {
			plan.Env = &project.Resolved{Vars: cliVars}
		}
	} else {
		plan.ProjectName = lp.P.Info.Name
		envName, err := lp.P.SelectEnv(envFlag, suiteName)
		if err != nil {
			return nil, &app.AppError{ExitCode: 2, Cause: err}
		}
		plan.Env, err = lp.P.Resolve(envName, suiteName, cliVars, os.LookupEnv)
		if err != nil {
			return nil, &app.AppError{ExitCode: 2, Cause: err}
		}
	}

	var err error
	if suiteName != "" {
		plan.Runbooks, err = scenario.Select(lp.Root, lp.P.Suites[suiteName].Select)
		if err == nil && len(plan.Runbooks) == 0 {
			err = fmt.Errorf("スイート %q の条件に合う runbook がありません (runnora: ブロックを持つ runbook だけが対象です)", suiteName)
		}
	} else {
		plan.Runbooks, err = scenario.FromArgs(args, lp.Root)
	}
	if err != nil {
		return nil, &app.AppError{ExitCode: 2, Cause: err}
	}

	var vars map[string]string
	if plan.Env != nil {
		vars = plan.Env.Vars
	}
	if undefined := undefinedRunbookVars(plan.Runbooks, vars); len(undefined) > 0 {
		return nil, &app.AppError{ExitCode: 2, Cause: fmt.Errorf("runbook が未定義の変数を参照しています (runnora.yaml の vars、OS の環境変数、--var のどれかで定義するか、${NAME:-既定値} と書いてください):\n  %s", strings.Join(undefined, "\n  "))}
	}
	return plan, nil
}
