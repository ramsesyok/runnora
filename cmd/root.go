// Package cmd は CLI コマンドの定義をまとめるパッケージ。
//
// 各サブコマンドはファクトリ関数 new<Name>Cmd() で生成し、
// NewRootCmd が集約して cobra のコマンドツリーを構築する。
// これにより各コマンドの独立テストが可能になっている。
package cmd

import (
	"github.com/spf13/cobra"
)

// NewRootCmd はルートコマンドを生成して返す。
// すべてのサブコマンドをここで登録する。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "runnora",
		Short: "WebAPI/gRPC シナリオテストツール",
		Long:  "runnora は runn を組み込んだ WebAPI/gRPC シナリオテストツールです。",
	}

	root.AddCommand(newRunCmd())      // runbook を実行する
	root.AddCommand(newVersionCmd())  // バージョン情報を表示する
	root.AddCommand(newListCmd())     // runbook を一覧表示する
	root.AddCommand(newCoverageCmd()) // OpenAPI/gRPC カバレッジを計測する
	root.AddCommand(newLoadtCmd())    // 負荷テストを実行する
	root.AddCommand(newNewCmd())      // runbook を新規作成またはステップを追加する
	root.AddCommand(newRprofCmd())    // 実行プロファイルを表示する

	return root
}

// Execute はルートコマンドを実行する。
// main.go から呼ばれる唯一のエントリポイント。
func Execute() error {
	return NewRootCmd().Execute()
}
