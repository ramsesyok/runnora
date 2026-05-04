package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultConfigTemplate = `app:
  name: runnora

oracle:
  driver: oracle
  dsn: %q
  max_open_conns: 10
  max_idle_conns: 2
  conn_max_lifetime_sec: 300

runn:
  trace: false

hooks:
  common:
    before: []
    after: []

generate:
  openapi: ""
  out_dir: "."
  case_format: json
  case_style: bundled
  mode: shallow
  clean_generated: false
  emit_manifest: false
  runner_name: req

report:
  format: text
  output: ""
`

// newInitCmd は "runnora init" サブコマンドを生成する。
//
// DB を使わない runbook をすぐ実行できるよう、oracle.dsn は空のまま出力する。
// SQL フックを使う場合は --dsn で指定するか、生成後に config.yaml を編集する。
func newInitCmd() *cobra.Command {
	var (
		out   string
		dsn   string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "デフォルトの config.yaml を作成する",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := filepath.Clean(out)
			if !force {
				if _, err := os.Stat(p); err == nil {
					return fmt.Errorf("init: %s already exists (use --force to overwrite)", p)
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("init: stat %s: %w", p, err)
				}
			}

			if dir := filepath.Dir(p); dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("init: mkdir %s: %w", dir, err)
				}
			}

			content := fmt.Sprintf(defaultConfigTemplate, dsn)
			if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
				return fmt.Errorf("init: write %s: %w", p, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", p)
			return nil
		},
	}

	cmd.Flags().StringVar(&out, "out", "config.yaml", "出力先 config ファイルパス")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Oracle DSN (SQL フックを使う場合に指定)")
	cmd.Flags().BoolVar(&force, "force", false, "既存ファイルを上書きする")

	return cmd
}
