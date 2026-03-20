// runnora は runn を組み込んだ WebAPI/gRPC シナリオテストツールのエントリポイント。
//
// 処理フロー:
//  1. cmd.Execute() でコブラコマンドツリーを実行
//  2. エラーが返った場合、app.AppError であれば ExitCode を取り出して os.Exit に渡す
//  3. app.AppError 以外の汎用エラーは exit 1 とする
//
// 終了コード一覧:
//   - 0: 全成功
//   - 1: runbook 失敗
//   - 2: 設定・引数不正
//   - 3: DB 接続失敗
//   - 4: before/after フック失敗
//   - 5: レポート出力失敗
package main

import (
	"errors"
	"os"

	"github.com/ramsesyok/runnora/cmd"
	"github.com/ramsesyok/runnora/internal/app"
)

func main() {
	if err := run(); err != nil {
		// app.AppError は ExitCode を持つ。errors.As で unwrap しながら探す。
		var appErr *app.AppError
		if errors.As(err, &appErr) {
			os.Exit(appErr.ExitCode)
		}
		// AppError でない場合は一般的な失敗 (exit 1)
		os.Exit(1)
	}
}

// run はコマンド実行を委譲し、エラーを返す。
// main と分離することでテスト可能性を高めている。
func run() error {
	return cmd.Execute()
}
