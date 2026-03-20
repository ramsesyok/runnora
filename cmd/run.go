package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/config"
)

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

			runner := app.NewRunner()
			report, err := runner.Run(cmd.Context(), opts)

			if report != nil {
				// レポートを標準出力に書く
				fmt.Fprintf(cmd.OutOrStdout(), "Runbooks: %d, Passed: %d, Failed: %d\n",
					report.Total, report.Passed, report.Failed)
			}

			return err
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "./config.yaml", "設定ファイルパス")
	cmd.Flags().StringArrayVar(&beforeSQL, "before-sql", nil, "実行前 SQL/PL/SQL ファイル（複数指定可）")
	cmd.Flags().StringArrayVar(&afterSQL, "after-sql", nil, "実行後 SQL/PL/SQL ファイル（複数指定可）")
	cmd.Flags().StringVar(&reportFormat, "report-format", "text", "レポート形式 (text|json|junit)")
	cmd.Flags().StringVar(&reportOut, "report-out", "", "レポート出力先ファイル（省略時は標準出力）")
	cmd.Flags().BoolVar(&trace, "trace", false, "トレースモードを有効にする")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "最初の失敗で停止する")

	return cmd
}
