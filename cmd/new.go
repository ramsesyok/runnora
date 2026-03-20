package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/k1LoW/runn"
	"github.com/spf13/cobra"
)

// newNewCmd は "runnora new" (alias: "append") サブコマンドを生成する。
//
// コマンドライン引数からステップを生成して runbook を作成または更新する。
// 出力先ファイルが既存の場合は追記モードになる。
//
// 使用例:
//
//	runnora new GET https://example.com/health          # 新規作成して stdout に出力
//	runnora new --out ./runbooks/foo.yml GET https://...  # ファイルに保存
//	runnora append --out ./runbooks/foo.yml POST https://... # 既存に追記
//	runnora new --out /tmp/smoke.yml --and-run GET https://... # 作成後すぐ実行
func newNewCmd() *cobra.Command {
	var (
		desc   string
		out    string
		andRun bool
	)

	cmd := &cobra.Command{
		Use:     "new [STEP_COMMAND ...]",
		Short:   "新しい runbook を作成またはステップを追加する",
		Aliases: []string{"append"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// 新規 runbook を生成する (desc は省略可)
			rb := runn.NewRunbook(desc)

			// 出力先が既存ファイルなら ParseRunbook で読み込んで追記モードにする
			if out != "" {
				p := filepath.Clean(out)
				if _, err := os.Stat(p); err == nil {
					// ファイルが存在する場合は既存 runbook をベースにする
					f, err := os.Open(p)
					if err != nil {
						return fmt.Errorf("new: open %s: %w", p, err)
					}
					defer f.Close()
					existing, err := runn.ParseRunbook(f)
					if err != nil {
						return fmt.Errorf("new: parse runbook: %w", err)
					}
					rb = existing
					// --desc が指定されていれば説明を上書きする
					if desc != "" {
						rb.Desc = desc
					}
				}
			}

			// 位置引数からステップを追加する。
			// 例: "GET https://example.com/hello" → HTTPステップを生成
			if len(args) > 0 {
				if err := rb.AppendStep(args...); err != nil {
					return fmt.Errorf("new: append step: %w", err)
				}
			}

			// runbook を YAML にシリアライズする
			b, err := yaml.Marshal(rb)
			if err != nil {
				return fmt.Errorf("new: marshal: %w", err)
			}

			if out == "" {
				// --out 未指定なら stdout に出力
				fmt.Fprint(cmd.OutOrStdout(), string(b))
			} else {
				// ファイルに書き込む (パーミッション 0600: オーナーのみ読み書き)
				p := filepath.Clean(out)
				if err := os.WriteFile(p, b, 0o600); err != nil {
					return fmt.Errorf("new: write %s: %w", p, err)
				}
			}

			// --and-run が指定されていれば保存した runbook をそのまま実行する
			if andRun {
				if out == "" {
					return fmt.Errorf("new: --and-run requires --out")
				}
				op, err := runn.Load(filepath.Clean(out), runn.Scopes("read:parent"))
				if err != nil {
					return fmt.Errorf("new: load for run: %w", err)
				}
				if err := op.RunN(cmd.Context()); err != nil {
					return fmt.Errorf("new: run: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&desc, "desc", "", "runbook の説明")
	cmd.Flags().StringVar(&out, "out", "", "出力先ファイルパス（省略時は標準出力）")
	cmd.Flags().BoolVar(&andRun, "and-run", false, "作成後すぐに実行する (--out が必要)")

	return cmd
}
