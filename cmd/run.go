package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/reporter"
)

// newRunCmd は "runnora run" サブコマンドを生成する。
//
// 処理の流れ:
//  1. CLI フラグを RunOptions に詰める
//  2. app.Runner.Run でフック実行 → runn 実行 → 結果集計
//  3. reporter でレポートを出力 (stdout またはファイル)
//  4. エラーを返すと main が ExitCode を解決して os.Exit を呼ぶ
func newRunCmd() *cobra.Command {
	var (
		configPath   string
		beforeSQL    []string
		afterSQL     []string
		reportFormat string
		reportOut    string
		trace        bool
		failFast     bool
	)

	cmd := &cobra.Command{
		Use:   "run [options] <runbook...>",
		Short: "runbook を実行する",
		RunE: func(cmd *cobra.Command, args []string) error {
			// CLI フラグを RunOptions に変換する。
			// runbook パスは位置引数 (args) から受け取る。
			opts := &config.RunOptions{
				ConfigPath:     configPath,
				BeforeSQLFiles: beforeSQL,
				AfterSQLFiles:  afterSQL,
				RunbookPaths:   args,
				ReportFormat:   reportFormat,
				ReportOutput:   reportOut,
				Trace:          trace,
				FailFast:       failFast,
			}

			// app.Runner に実行を委譲する。
			// RunOptions が空の場合や DB 接続失敗は AppError として返る。
			runner := app.NewRunner()
			report, err := runner.Run(cmd.Context(), opts)

			// report != nil なら runbook は少なくとも一部実行されている。
			// エラーがあっても先にレポートを出力し、その後エラーを返す。
			if report != nil {
				// 出力先が指定されていればファイルへ書く
				var rep reporter.Reporter
				if reportOut != "" {
					r, rerr := reporter.NewFileReporter(reportFormat, reportOut)
					if rerr != nil {
						return fmt.Errorf("report: %w", rerr)
					}
					defer r.Close()
					rep = r
				} else {
					// 指定なしは stdout
					rep = reporter.NewTextReporter(cmd.OutOrStdout())
				}
				if werr := rep.Write(report); werr != nil {
					return fmt.Errorf("report write: %w", werr)
				}
			}

			return err
		},
	}

	// --- フラグ定義 ---
	cmd.Flags().StringVar(&configPath, "config", "./config.yaml", "設定ファイルパス")
	// StringArrayVar は --before-sql を複数回指定できる
	cmd.Flags().StringArrayVar(&beforeSQL, "before-sql", nil, "実行前 SQL/PL/SQL ファイル（複数指定可）")
	cmd.Flags().StringArrayVar(&afterSQL, "after-sql", nil, "実行後 SQL/PL/SQL ファイル（複数指定可）")
	cmd.Flags().StringVar(&reportFormat, "report-format", "text", "レポート形式 (text|json|junit)")
	cmd.Flags().StringVar(&reportOut, "report-out", "", "レポート出力先ファイル（省略時は標準出力）")
	cmd.Flags().BoolVar(&trace, "trace", false, "トレースモードを有効にする")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "最初の失敗で停止する")

	return cmd
}
