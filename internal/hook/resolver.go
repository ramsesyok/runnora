// Package hook は before/after フックとして SQL/PL/SQL ファイルを実行する機能を提供する。
//
// フックの役割:
//   - Before フック: runbook 実行前に DB の状態を整備する (テーブル TRUNCATE、テストデータ投入など)
//   - After フック:  runbook 実行後に DB の状態をクリーンアップする (ロールバック、後始末など)
//
// フックファイルは oracle.Executor を通じて実行されるため、
// 実際のフック実行コードは Oracle DB に依存しない (インターフェース経由)。
// テスト時は stub の Executor を渡すことで Oracle 不要になる。
package hook

import (
	"fmt"
	"os"
	"strings"
)

// Resolver はフック SQL ファイルの存在を事前に検証する。
//
// 設計の意図:
//   runbook 実行を開始する前にフックファイルの存在を検証することで、
//   実行の途中でファイルが見つからないエラーが発生するのを防ぐ。
//   Fail-fast (早期失敗) の原則に従い、実行前に全ての問題を列挙して報告する。
type Resolver struct{}

// NewResolver は Resolver を生成する。
func NewResolver() *Resolver {
	return &Resolver{}
}

// Validate は指定されたすべてのファイルが存在することを確認する。
//
// 動作:
//   - ファイルが全て存在する場合は nil を返す
//   - 存在しないファイルが 1 つ以上ある場合は、欠損パスを全て列挙したエラーを返す
//     (最初のエラーで止まらず、全て確認してからまとめて報告する)
//
// 使用例:
//
//	resolver := hook.NewResolver()
//	if err := resolver.Validate(beforeFiles); err != nil {
//	    // err には見つからなかった全ファイルパスが含まれる
//	    return nil, &AppError{ExitCode: 4, Cause: err}
//	}
func (r *Resolver) Validate(files []string) error {
	var missing []string
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		// 全欠損ファイルをカンマ区切りで列挙してエラーメッセージに含める。
		// これにより、ユーザーは 1 回のエラーで全ての問題を把握できる。
		return fmt.Errorf("hook: missing SQL files: %s", strings.Join(missing, ", "))
	}
	return nil
}
