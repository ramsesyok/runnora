package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/k1LoW/runn"
	"github.com/spf13/cobra"
)

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
			rb := runn.NewRunbook(desc)

			// 出力先が既存ファイルなら ParseRunbook で読み込んで追記モードにする
			if out != "" {
				p := filepath.Clean(out)
				if _, err := os.Stat(p); err == nil {
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
					if desc != "" {
						rb.Desc = desc
					}
				}
			}

			if len(args) > 0 {
				if err := rb.AppendStep(args...); err != nil {
					return fmt.Errorf("new: append step: %w", err)
				}
			}

			b, err := yaml.Marshal(rb)
			if err != nil {
				return fmt.Errorf("new: marshal: %w", err)
			}

			if out == "" {
				fmt.Fprint(cmd.OutOrStdout(), string(b))
			} else {
				p := filepath.Clean(out)
				if err := os.WriteFile(p, b, 0o600); err != nil {
					return fmt.Errorf("new: write %s: %w", p, err)
				}
			}

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
	cmd.Flags().BoolVar(&andRun, "and-run", false, "作成後すぐに実行する")

	return cmd
}
