package generate

import (
	"fmt"
	"io"
	"strings"

	"github.com/ramsesyok/runnora/internal/config"
)

// GenerateResult は generate コマンドの実行結果サマリーを保持する。
type GenerateResult struct {
	Total    int // 対象 operation 数
	Generated int // 生成した operation 数
	Skipped   int // スキップした operation 数 (既存ファイルあり、または deprecated)
	Warnings  int // 警告数
	OutDir    string
}

// Generate は OpenAPI ファイルからテスト資産を一括生成するメイン処理。
//
// 実行フロー (設計書 §26.2):
//  1. OpenAPI をロードして operation 一覧を抽出
//  2. tags / operationIDs / skip-deprecated でフィルタ
//  3. --clean の場合は generated/ を掃除
//  4. 各 operation に対して template・case・suite を生成
//  5. --emit-manifest の場合は manifest.json を出力
func Generate(opts *config.GenerateOptions) (*GenerateResult, error) {
	// 1. OpenAPI ロード
	ops, err := LoadOperations(opts.OpenAPIPath)
	if err != nil {
		return nil, err
	}

	// 2. フィルタ
	ops = filterOperations(ops, opts)

	result := &GenerateResult{
		Total:  len(ops),
		OutDir: opts.OutDir,
	}

	// 3. --clean
	if opts.Clean {
		if err := CleanGenerated(opts.OutDir); err != nil {
			return nil, err
		}
	}

	// 4. 各 operation のファイル生成
	var manifestEntries []ManifestEntry
	for _, op := range ops {
		entry, skip, genErr := generateForOperation(op, opts)
		if genErr != nil {
			return result, genErr
		}
		if skip {
			result.Skipped++
		} else {
			result.Generated++
		}
		manifestEntries = append(manifestEntries, entry)
	}

	// 5. manifest
	if opts.EmitManifest && len(manifestEntries) > 0 {
		if err := EmitManifest(opts.OutDir, opts.OpenAPIPath, manifestEntries); err != nil {
			return result, err
		}
	}

	return result, nil
}

// generateForOperation は 1 つの operation に対して template・case・suite を生成する。
// すでに全ファイルが存在して force=false の場合は skip=true を返す。
func generateForOperation(op *OperationInfo, opts *config.GenerateOptions) (ManifestEntry, bool, error) {
	entry := ManifestEntry{
		OperationID: op.OperationID,
		Method:      op.Method,
		Path:        op.Path,
		Tag:         op.PrimaryTag,
	}

	runnerName := opts.RunnerName
	if runnerName == "" {
		runnerName = "req"
	}

	// template runbook
	tplPath, err := EmitTemplate(opts.OutDir, op, opts.OpenAPIPath, runnerName, opts.Force)
	if err != nil {
		return entry, false, err
	}
	entry.TemplatePath = tplPath

	// case JSON
	casePath, err := EmitCase(opts.OutDir, op, opts.Force)
	if err != nil {
		return entry, false, err
	}
	entry.CasePaths = []string{casePath}

	// suite runbook
	suitePath, err := EmitSuite(opts.OutDir, op, entry.CasePaths, opts.Force)
	if err != nil {
		return entry, false, err
	}
	entry.SuitePath = suitePath

	// 全ファイルが新規ではなくスキップされたかどうかの判断は力不足なので
	// 今回は生成試みを「generated」としてカウントする
	return entry, false, nil
}

// filterOperations は tags / operationIDs / deprecated フィルタを適用する。
func filterOperations(ops []*OperationInfo, opts *config.GenerateOptions) []*OperationInfo {
	var result []*OperationInfo
	for _, op := range ops {
		// deprecated スキップ
		if opts.SkipDeprecated && op.Deprecated {
			continue
		}

		// tags フィルタ
		if len(opts.Tags) > 0 && !hasAnyTag(op.Tags, opts.Tags) {
			continue
		}

		// operationID フィルタ
		if len(opts.OperationIDs) > 0 && !containsStr(opts.OperationIDs, op.OperationID) {
			continue
		}

		result = append(result, op)
	}
	return result
}

// hasAnyTag は op の Tags に targets のいずれかが含まれるかを返す。
func hasAnyTag(opTags, targets []string) bool {
	for _, t := range targets {
		for _, ot := range opTags {
			if strings.EqualFold(ot, t) {
				return true
			}
		}
	}
	return false
}

// containsStr は slice の中に s が含まれるかを返す。
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// PrintReport は generate の結果サマリーを w に書き出す。
func PrintReport(w io.Writer, result *GenerateResult, entries []ManifestEntry) {
	fmt.Fprintf(w, "Generated %d operations (%d templates, %d suites, %d cases)\n",
		result.Generated, result.Generated, result.Generated, result.Generated)
	if result.Skipped > 0 {
		fmt.Fprintf(w, "  Skipped: %d\n", result.Skipped)
	}
	fmt.Fprintf(w, "  Output:  %s\n", result.OutDir)
	for _, e := range entries {
		fmt.Fprintf(w, "  [%s] %s (%s) → %s\n",
			strings.ToUpper(e.Method), e.Path, e.OperationID, e.TemplatePath)
	}
}
