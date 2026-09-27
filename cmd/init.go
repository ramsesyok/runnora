package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultProjectTemplate = `# runnora プロジェクトファイル。パスはこのファイルのあるディレクトリを基準にする。
version: 2

project:
  name: runnora

defaults:
  env: local                 # --env を省略したときの環境

environments:
  local:
    description: ローカル環境
    vars:                    # runbook から ${NAME} で参照する。同名の OS の環境変数や --var が優先
      RUNNORA_BASE_URL: http://localhost:8080
%s
# suites:                    # runnora run --suite <名前> で実行する runbook の選び方
#   scenarios:
#     select:
#       paths: [runbooks/scenarios/*.yml]   # runnora: ブロックを持つ runbook だけが対象

runn:
  scopes: []                 # runn に追加で許可するスコープ (例: run:exec)
  trace: false

report:
  format: text
  output: ""

generate:
  openapi: ""
  out_dir: "."
  case_format: json
  case_style: bundled
  mode: shallow
  clean_generated: false
  emit_manifest: false
  runner_name: req
`

// oracleSectionWithDSN は --dsn を指定したときの oracle / hooks セクション。
const oracleSectionWithDSN = `    oracle:                  # SQL の前後処理 (hooks と runbook の runnora: ブロック) で使う接続
      dsn: %q
    hooks:                   # この環境の全シナリオに共通する前後処理
      before: []
      after: []`

// oracleSectionComment は --dsn を指定しないときの oracle / hooks セクション (コメント)。
const oracleSectionComment = `    # SQL の前後処理を使う場合は oracle を設定する
    # oracle:
    #   dsn: ${ORACLE_DSN}
    # hooks:
    #   before: []
    #   after: []`

// newInitCmd は "runnora init" サブコマンドを生成する。
//
// DB を使わない runbook をすぐ実行できるよう、既定では oracle をコメントにして出力する。
// SQL フックを使う場合は --dsn で指定するか、生成後に runnora.yaml を編集する。
func newInitCmd() *cobra.Command {
	var (
		out   string
		dsn   string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "runnora.yaml (プロジェクトファイル) の雛形を作成する",
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

			oracleSection := oracleSectionComment
			if dsn != "" {
				oracleSection = fmt.Sprintf(oracleSectionWithDSN, dsn)
			}
			content := fmt.Sprintf(defaultProjectTemplate, oracleSection)
			if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
				return fmt.Errorf("init: write %s: %w", p, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", p)
			return nil
		},
	}

	cmd.Flags().StringVar(&out, "out", "runnora.yaml", "出力先ファイルパス")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Oracle DSN (SQL フックを使う場合に指定)")
	cmd.Flags().BoolVar(&force, "force", false, "既存ファイルを上書きする")

	return cmd
}
