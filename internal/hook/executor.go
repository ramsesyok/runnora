package hook

import (
	"context"
	"fmt"

	"github.com/ramsesyok/runnora/internal/oracle"
)

// RunBefore は before フックのファイルリストを指定順序で順番に実行する。
//
// ファイルの実行順序は Order で決定済み:
//
//	環境の hooks.before → runbook の runnora.before の順に実行される。
//
// 最初のエラーで即座に停止し、エラーメッセージにはフェーズ名とファイルパスを含める。
// これにより、ログを見るだけでどのフックが失敗したかを特定できる。
//
// 引数:
//   - exec: SQL 実行器。oracle.OracleExecutor または テスト用 stub
//   - files: 実行する SQL/PL/SQL ファイルのパスリスト (順序が重要)
func RunBefore(ctx context.Context, exec oracle.Executor, files []string) error {
	return runFiles(ctx, exec, "before", files, nil)
}

// OnFile はフックのファイルを 1 つ実行するたびに呼ばれる (レポートへの記録用)。
// err は成功なら nil。失敗したファイルの後のファイルは実行しないので呼ばれない。
type OnFile func(phase, file string, err error)

// RunBeforeWith は RunBefore と同じく実行し、ファイルごとの結果を onFile に渡す。
func RunBeforeWith(ctx context.Context, exec oracle.Executor, files []string, onFile OnFile) error {
	return runFiles(ctx, exec, "before", files, onFile)
}

// RunAfterWith は RunAfter と同じく実行し、ファイルごとの結果を onFile に渡す。
func RunAfterWith(ctx context.Context, exec oracle.Executor, files []string, onFile OnFile) error {
	return runFiles(ctx, exec, "after", files, onFile)
}

// RunAfter は after フックのファイルリストを指定順序で順番に実行する。
//
// ファイルの実行順序は Order で決定済み:
//
//	runbook の runnora.after → 環境の hooks.after の順に実行される。
//
// シナリオ固有のクリーンアップを先に行い、
// その後に共通クリーンアップを実行するという設計。
//
// 引数:
//   - exec: SQL 実行器。oracle.OracleExecutor または テスト用 stub
//   - files: 実行する SQL/PL/SQL ファイルのパスリスト (順序が重要)
func RunAfter(ctx context.Context, exec oracle.Executor, files []string) error {
	return runFiles(ctx, exec, "after", files, nil)
}

// Order は 1 つの runbook で実行する前後処理のファイル順を組み立てる。
//
//	before: 環境の hooks.before → runbook の runnora.before
//	after:  runbook の runnora.after → 環境の hooks.after
//
// 共通のセットアップの後にシナリオ固有のセットアップを行い、
// シナリオ固有のクリーンアップの後に共通のクリーンアップを行う。
func Order(envBefore, envAfter, scenarioBefore, scenarioAfter []string) (before, after []string) {
	before = append(append([]string{}, envBefore...), scenarioBefore...)
	after = append(append([]string{}, scenarioAfter...), envAfter...)
	return before, after
}

// runFiles は RunBefore / RunAfter の共通実装。
//
// フェーズ名 (before/after) をエラーメッセージに含めることで、
// どちらのフックが失敗したかをユーザーが識別できる。
//
// エラーメッセージのフォーマット例:
//
//	"hook before ./sql/setup.sql: oracle: exec: ORA-00942: table or view does not exist"
func runFiles(ctx context.Context, exec oracle.Executor, phase string, files []string, onFile OnFile) error {
	for _, f := range files {
		err := exec.ExecFile(ctx, f)
		if onFile != nil {
			onFile(phase, f, err)
		}
		if err != nil {
			// エラーにフェーズ名とファイルパスを付加して返す。
			// %w を使ってラップすることで、呼び出し元が errors.Is/As で原因を検査できる。
			return fmt.Errorf("hook %s %s: %w", phase, f, err)
		}
	}
	return nil
}
