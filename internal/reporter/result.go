// Package reporter は runbook の実行結果集計と出力を担当する。
//
// 設計方針:
//   - Report / RunResult は純粋なデータ構造 (ロジックなし)
//   - Reporter インターフェースを通じて出力先・形式を切り替えられる
//   - text.go に TextReporter と fileReporter を実装
//
// 出力例 (TextReporter):
//
//	Runbooks: 3, Passed: 2, Failed: 1
//	  FAIL: ./runbooks/user_create.yml
//	    Error: assert failed: steps.check.res.status == 200
package reporter

// Report は runbook 実行結果の集計を保持する。
//
// フィールド:
//   - Total:   実行した runbook の総数 (Passed + Failed)
//   - Passed:  成功した runbook 数
//   - Failed:  失敗した runbook 数 (フック失敗 + runbook 失敗の合計)
//   - Results: 各 runbook の詳細結果リスト (成功・失敗ともに含む)
//
// 利用例:
//
//	rep := reporter.NewTextReporter(os.Stdout)
//	rep.Write(report)
type Report struct {
	Total   int
	Passed  int
	Failed  int
	Results []RunResult
}

// RunResult は 1 つの runbook の実行結果を保持する。
//
// フィールド:
//   - Path:   runbook ファイルのパス (例: "./runbooks/user_create.yml")
//   - Passed: true なら成功、false なら失敗
//   - Error:  失敗時のエラーメッセージ。成功時は空文字列
//
// Error フィールドには runn が生成するエラーメッセージが入る。
// 例: "assert failed: steps.check.res.status == 200"
//     "hook before ./sql/setup.sql: oracle: exec: ORA-00942"
type RunResult struct {
	Path   string
	Passed bool
	Error  string
}
