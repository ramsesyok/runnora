package hook

import (
	"context"
	"fmt"

	"github.com/ramsesyok/runnora/internal/oracle"
)

// RunBefore は before フックのファイルリストを指定順序で順番に実行する。
//
// ファイルの実行順序は config.BuildBeforeFiles で決定済み:
//   config の common.before → CLI --before-sql の順に実行される。
//
// 最初のエラーで即座に停止し、エラーメッセージにはフェーズ名とファイルパスを含める。
// これにより、ログを見るだけでどのフックが失敗したかを特定できる。
//
// 引数:
//   - exec: SQL 実行器。oracle.OracleExecutor または テスト用 stub
//   - files: 実行する SQL/PL/SQL ファイルのパスリスト (順序が重要)
func RunBefore(ctx context.Context, exec oracle.Executor, files []string) error {
	return runFiles(ctx, exec, "before", files)
}

// RunAfter は after フックのファイルリストを指定順序で順番に実行する。
//
// ファイルの実行順序は config.BuildAfterFiles で決定済み:
//   CLI --after-sql → config の common.after の順に実行される。
//
// テスト固有のクリーンアップ (--after-sql) を先に行い、
// その後に共通クリーンアップ (common.after) を実行するという設計。
//
// 引数:
//   - exec: SQL 実行器。oracle.OracleExecutor または テスト用 stub
//   - files: 実行する SQL/PL/SQL ファイルのパスリスト (順序が重要)
func RunAfter(ctx context.Context, exec oracle.Executor, files []string) error {
	return runFiles(ctx, exec, "after", files)
}

// runFiles は RunBefore / RunAfter の共通実装。
//
// フェーズ名 (before/after) をエラーメッセージに含めることで、
// どちらのフックが失敗したかをユーザーが識別できる。
//
// エラーメッセージのフォーマット例:
//
//	"hook before ./sql/setup.sql: oracle: exec: ORA-00942: table or view does not exist"
func runFiles(ctx context.Context, exec oracle.Executor, phase string, files []string) error {
	for _, f := range files {
		if err := exec.ExecFile(ctx, f); err != nil {
			// エラーにフェーズ名とファイルパスを付加して返す。
			// %w を使ってラップすることで、呼び出し元が errors.Is/As で原因を検査できる。
			return fmt.Errorf("hook %s %s: %w", phase, f, err)
		}
	}
	return nil
}
