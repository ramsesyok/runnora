package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/k1LoW/runn"
	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/app"
)

// newCoverageCmd は "runnora coverage" サブコマンドを生成する。
//
// runbook が OpenAPI 3 スペックや gRPC proto のどのエンドポイントを
// カバーしているかを計測して表示する。
// runn.CollectCoverage がスペックと runbook の対応関係を解析する。
func newCoverageCmd() *cobra.Command {
	var (
		long        bool
		format      string
		projectPath string
		envName     string
		suiteName   string
		vars        []string
	)

	cmd := &cobra.Command{
		Use:   "coverage [options] [runbook...]",
		Short: "OpenAPI / gRPC のカバレッジを表示する",
		Long: `runbook が OpenAPI / gRPC のどのエンドポイントを呼んでいるかを表示する (runbook は実行しない)。

対象の runbook は、引数 (glob 可) で指定するか、--suite で runnora.yaml のスイートを指定する。
runners の接続先などの ${VAR} は、run と同じく runnora.yaml の環境 (--env)、OS の環境変数、--var で展開する。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if suiteName != "" && len(args) > 0 {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("--suite と runbook の引数は同時に指定できません")}
			}
			if suiteName == "" && len(args) == 0 {
				return &app.AppError{ExitCode: 2, Cause: fmt.Errorf("runbook を指定するか --suite を指定してください")}
			}
			cmd.SilenceUsage = true
			cliVars, err := parseVars(vars)
			if err != nil {
				return &app.AppError{ExitCode: 2, Cause: err}
			}
			lp, err := loadProject(projectPath, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			// run と同じ規則で runbook と変数を決める。runn は runners の ${VAR} を
			// プロセスの環境変数で展開するため、変数表を環境変数に設定してから読み込む
			// (設定しないと接続先が空になり、runbook が黙って読み飛ばされる)。
			plan, err := buildRunPlan(lp, envName, suiteName, cliVars, args)
			if err != nil {
				return err
			}
			if plan.Env != nil {
				for k, v := range plan.Env.Vars {
					if err := os.Setenv(k, v); err != nil {
						return &app.AppError{ExitCode: 2, Cause: err}
					}
				}
			}
			paths := make([]string, 0, len(plan.Runbooks))
			for _, rb := range plan.Runbooks {
				paths = append(paths, rb.Path)
			}
			pathp := strings.Join(paths, string(filepath.ListSeparator))

			opts := []runn.Option{
				runn.LoadOnly(),            // 実行しない
				runn.Scopes("read:parent"), // 親ディレクトリ読み取りを許可
			}

			op, err := runn.Load(pathp, opts...)
			if err != nil {
				return fmt.Errorf("coverage: load: %w", err)
			}

			// CollectCoverage は runbook が参照している OpenAPI/gRPC スペックを
			// 解析し、各エンドポイントへのアクセス回数を集計する
			cov, err := op.CollectCoverage(cmd.Context())
			if err != nil {
				return fmt.Errorf("coverage: collect: %w", err)
			}

			// JSON 形式出力
			if format == "json" {
				b, err := json.MarshalIndent(cov, "", "  ")
				if err != nil {
					return fmt.Errorf("coverage: json: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}

			// テキスト形式出力
			w := cmd.OutOrStdout()
			for _, spec := range cov.Specs {
				fmt.Fprintf(w, "Spec: %s\n", spec.Key)
				if long {
					// --long: エンドポイントごとにアクセス回数を表示
					for endpoint, count := range spec.Coverages {
						fmt.Fprintf(w, "  %-60s  %d\n", endpoint, count)
					}
				} else {
					// 通常: カバー済み/全体 と割合のみ表示
					total := len(spec.Coverages)
					covered := 0
					for _, count := range spec.Coverages {
						if count > 0 {
							covered++
						}
					}
					if total > 0 {
						fmt.Fprintf(w, "  covered: %d / %d (%.1f%%)\n",
							covered, total, float64(covered)/float64(total)*100)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&long, "long", "l", false, "エンドポイントごとの詳細カバレッジを表示する")
	cmd.Flags().StringVar(&format, "format", "", "出力形式 (json)")
	cmd.Flags().StringVar(&projectPath, "project", "", "runnora.yaml のパス (省略時はカレントディレクトリから親へ探す)")
	cmd.Flags().StringVar(&envName, "env", "", "変数を展開する環境 (runnora.yaml の environments の名前)")
	cmd.Flags().StringVar(&suiteName, "suite", "", "対象のスイート (runnora.yaml の suites の名前)")
	cmd.Flags().StringArrayVar(&vars, "var", nil, "変数の上書き NAME=VALUE (複数指定可)")

	return cmd
}
