package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/k1LoW/runn"
	"github.com/spf13/cobra"
)

// newCoverageCmd は "runnora coverage" サブコマンドを生成する。
//
// runbook が OpenAPI 3 スペックや gRPC proto のどのエンドポイントを
// カバーしているかを計測して表示する。
// runn.CollectCoverage がスペックと runbook の対応関係を解析する。
func newCoverageCmd() *cobra.Command {
	var (
		long   bool
		format string
	)

	cmd := &cobra.Command{
		Use:   "coverage [PATH_PATTERN ...]",
		Short: "OpenAPI / gRPC のカバレッジを表示する",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathp := strings.Join(args, string(filepath.ListSeparator))

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

	return cmd
}
