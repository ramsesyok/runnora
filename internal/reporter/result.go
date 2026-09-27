// Package reporter は runbook の実行結果集計と出力を担当する。
//
// 設計方針:
//   - Report / RunResult は純粋なデータ構造 (ロジックなし)
//   - Reporter インターフェースを通じて出力先・形式を切り替えられる
//   - text / json / junit の各 Reporter を提供
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
//   - Total:   対象の runbook の総数 (Passed + Failed + Skipped)
//   - Passed:  期待どおりだった runbook 数 (期待どおりの失敗を含む)
//   - Failed:  期待どおりでなかった runbook 数
//   - Skipped: 環境の対象外で実行しなかった runbook 数
//   - Results: 各 runbook の詳細結果リスト (成功・失敗ともに含む)
//
// 利用例:
//
//	rep := reporter.NewTextReporter(os.Stdout)
//	rep.Write(report)
type Report struct {
	// Project は runnora.yaml の project.name。
	Project string `json:"project,omitempty"`
	// Env は実行した環境。環境を使わない場合は nil。
	Env *EnvInfo `json:"env,omitempty"`
	// Suite は実行したスイート名。runbook を直接指定した場合は空。
	Suite string `json:"suite,omitempty"`
	// EvidenceDir は証跡のフォルダ (実行ごとのフォルダからの相対パス、外なら絶対パス)。
	EvidenceDir string `json:"evidenceDir,omitempty"`
	// Backends は環境に宣言された裏のサービスの扱い (記録用)。
	Backends map[string]Backend `json:"backends,omitempty"`

	Total   int         `json:"total"`
	Passed  int         `json:"passed"`
	Failed  int         `json:"failed"`
	Skipped int         `json:"skipped,omitempty"`
	Results []RunResult `json:"results"`
}

// EnvInfo は実行した環境の情報。
type EnvInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Overrides は runnora.yaml の vars を OS の環境変数か --var が上書きした変数の名前 (値は載せない)。
	Overrides []string `json:"overrides,omitempty"`
}

// Backend は裏のサービスの扱い。
type Backend struct {
	Mode string `json:"mode"`
	Note string `json:"note,omitempty"`
}

// RunResult は 1 つの runbook の実行結果を保持する。
//
// フィールド:
//   - ID:     シナリオ ID (runnora: ブロックの id、なければパスから作った ID)
//   - Path:   runbook ファイルのパス
//   - Expect: 期待した結果 (pass / fail / hookFail)
//   - Actual: 実際の結果 (pass / fail / hookFail / skipped)
//   - Passed: 期待どおりだったか。skipped の場合は true
//   - Error:  runbook やフックが失敗したときのエラーメッセージ
//
// Error フィールドには runn が生成するエラーメッセージが入る。
// 例: "assert failed: steps.check.res.status == 200"
//
//	"hook before ./sql/setup.sql: oracle: exec: ORA-00942"
type RunResult struct {
	ID     string `json:"id,omitempty"`
	Path   string `json:"path"`
	Expect string `json:"expect,omitempty"`
	Actual string `json:"actual,omitempty"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
	// Evidence は保存した証跡ファイル (実行順)。Path は Report.EvidenceDir からの相対パス。
	Evidence []EvidenceFile `json:"evidence,omitempty"`
}

// EvidenceFile は 1 ステップ分の証跡ファイル。
type EvidenceFile struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

// Name はレポートで runbook を表す名前 (ID、なければパス) を返す。
func (r RunResult) Name() string {
	if r.ID != "" {
		return r.ID
	}
	return r.Path
}
