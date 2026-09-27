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
//
// SchemaVersion は report.json の形の版。ステップ単位の項目を加えたものが 2。
const SchemaVersion = 2

type Report struct {
	// SchemaVersion は report.json の形の版 (SchemaVersion)。
	SchemaVersion int `json:"schemaVersion"`
	// Runnora は実行した runnora のバージョン。
	Runnora string `json:"runnora,omitempty"`
	// StartedAt は実行を始めた日時 (RFC 3339)。ElapsedMs は全体の所要時間。
	StartedAt string `json:"startedAt,omitempty"`
	ElapsedMs int64  `json:"elapsedMs"`
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
	ID        string `json:"id,omitempty"`
	Desc      string `json:"desc,omitempty"`
	Path      string `json:"path"`
	Expect    string `json:"expect,omitempty"`
	Actual    string `json:"actual,omitempty"`
	Passed    bool   `json:"passed"`
	Error     string `json:"error,omitempty"`
	ElapsedMs int64  `json:"elapsedMs,omitempty"`
	// Hooks は実行した前後処理 (実行順)。失敗したファイルの後のファイルは実行しないので載らない。
	Hooks []HookResult `json:"hooks,omitempty"`
	// Steps はステップの結果 (実行順。include 先を平らに並べる)。
	Steps []StepResult `json:"steps,omitempty"`
}

// HookResult は前後処理の SQL ファイル 1 つの結果。
type HookResult struct {
	Phase string `json:"phase"` // before / after
	File  string `json:"file"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ステップの結果 (StepResult.Result)。
const (
	StepSuccess = "success"
	StepFailure = "failure"
	// StepSkipped は runn の if で飛ばしたステップ。
	StepSkipped = "skipped"
	// StepNotRun は前のステップの失敗で実行しなかったステップ。
	StepNotRun = "notRun"
)

// StepResult はステップ 1 つの結果。キーは証跡のファイル名と docgen の manifest.json と共通。
type StepResult struct {
	// Key はステップのキー。include 先は . でつなぐ (inc.call)。loop の回は証跡の側で [n] を付ける。
	Key string `json:"key"`
	// Index はトップレベルのステップの番号 (1 始まり)。include 先は呼び出したステップの番号。
	Index     int    `json:"index"`
	Desc      string `json:"desc,omitempty"`
	Runner    string `json:"runner,omitempty"`
	Result    string `json:"result"`
	ElapsedMs int64  `json:"elapsedMs,omitempty"`
	Error     string `json:"error,omitempty"`
	// Evidence はこのステップの証跡ファイル (Report.EvidenceDir からの相対パス)。loop は回ごとに 1 つ。
	Evidence []string `json:"evidence,omitempty"`
}

// Name はレポートで runbook を表す名前 (ID、なければパス) を返す。
func (r RunResult) Name() string {
	if r.ID != "" {
		return r.ID
	}
	return r.Path
}
