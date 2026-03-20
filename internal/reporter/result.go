package reporter

// Report は runbook 実行結果の集計を保持する。
type Report struct {
	Total   int
	Passed  int
	Failed  int
	Results []RunResult
}

// RunResult は 1 つの runbook の実行結果を保持する。
type RunResult struct {
	Path   string
	Passed bool
	Error  string
}
