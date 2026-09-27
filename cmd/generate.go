package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/generate"
)

// newGenerateCmd は "runnora generate" サブコマンドを生成する。
//
// OpenAPI 定義ファイルから以下のテスト資産を生成する。
//  1. template runbook — 1 件の case を受け取って API を 1 回呼ぶ薄い runbook
//  2. case JSON       — request / expect の具体値を保持するファイル
//  3. suite runbook   — 複数 case を loop で回す runbook
//
// 処理フロー:
//  1. CLI フラグと設定ファイルをマージして GenerateOptions を構築
//  2. generate.Generate で資産を生成
//  3. サマリーを stdout に出力
func newGenerateCmd() *cobra.Command {
	var (
		projectPath         string
		openAPIPath         string
		outDir              string
		tagsStr             string
		operationIDsStr     string
		mode                string
		caseFormat          string
		caseStyle           string
		clean               bool
		force               bool
		skipDeprecated      bool
		server              string
		runnerName          string
		emitManifest        bool
		emitResponseExample bool
	)

	cmd := &cobra.Command{
		Use:   "generate [options]",
		Short: "OpenAPI 定義からテスト資産を生成する",
		Long: `OpenAPI 3.0.x / 3.1.x の定義ファイルから以下の 3 種類のテスト資産を生成する。

  1. template runbook  ... runbooks/generated/<tag>/<method>_<operationId>.template.yml
  2. case JSON         ... cases/generated/<tag>/<method>_<operationId>/default.json
  3. suite runbook     ... runbooks/generated/<tag>/<method>_<operationId>.suite.yml

生成物は再生成前提で手編集禁止。手編集が必要な runbook は
runbooks/evidence/ にコピーして育てる運用を推奨する。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 引数の解析が済んだ後のエラーでは使い方を表示しない (エラーが読みにくくなるため)
			cmd.SilenceUsage = true
			if err := rejectLegacyFlags(cmd); err != nil {
				return err
			}
			// runnora.yaml を読み込み、CLI フラグで上書きする
			lp, err := loadProject(projectPath, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			opts, err := buildGenerateOptions(
				lp, openAPIPath, outDir, tagsStr, operationIDsStr,
				mode, caseFormat, caseStyle, server, runnerName,
				clean, force, skipDeprecated, emitManifest, emitResponseExample,
			)
			if err != nil {
				return err
			}

			result, err := generate.Generate(opts)
			if err != nil {
				return fmt.Errorf("generate: %w", err)
			}

			// サマリーを stdout に出力する
			// manifest エントリは service 内で生成されているが、
			// ここでは result のみ使って簡易表示する
			generate.PrintReport(cmd.OutOrStdout(), result, nil)
			return nil
		},
	}

	// --- フラグ定義 ---
	cmd.Flags().StringVar(&projectPath, "project", "", "runnora.yaml のパス (省略時はカレントディレクトリから親へ探す)")
	cmd.Flags().StringVar(&openAPIPath, "openapi", "", "OpenAPI ファイルパス (YAML/JSON)")
	cmd.Flags().StringVar(&outDir, "out", "", "生成物の出力基底ディレクトリ (デフォルト: runnora.yaml の generate.out_dir、なければプロジェクトルート)")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "生成対象タグ (カンマ区切り): 例 users,orders")
	cmd.Flags().StringVar(&operationIDsStr, "operation-ids", "", "生成対象 operationId (カンマ区切り)")
	cmd.Flags().StringVar(&mode, "mode", "", "生成モード: shallow (デフォルト)")
	cmd.Flags().StringVar(&caseFormat, "case-format", "", "case ファイル形式: json (デフォルト)")
	cmd.Flags().StringVar(&caseStyle, "case-style", "", "case スタイル: bundled (デフォルト)")
	cmd.Flags().BoolVar(&clean, "clean", false, "生成前に generated/ ディレクトリを掃除する")
	cmd.Flags().BoolVar(&force, "force", false, "既存ファイルを強制上書きする")
	cmd.Flags().BoolVar(&skipDeprecated, "skip-deprecated", false, "deprecated な operation をスキップする")
	cmd.Flags().StringVar(&server, "server", "", "template runbook の endpoint として使う server URL")
	cmd.Flags().StringVar(&runnerName, "runner-name", "", "template runbook のランナー名 (デフォルト: req)")
	cmd.Flags().BoolVar(&emitManifest, "emit-manifest", false, "manifest.json を生成する")
	cmd.Flags().BoolVar(&emitResponseExample, "emit-response-example", false, "(非推奨) レスポンス example は常に case に含まれるため効果なし")
	_ = cmd.Flags().MarkDeprecated("emit-response-example", "レスポンス example は指定しなくても常に case JSON の expect.body に含まれます。今後のリリースで削除します")
	addLegacyFlags(cmd, "config")

	return cmd
}

// buildGenerateOptions は runnora.yaml の generate セクションと CLI フラグをマージして GenerateOptions を返す。
//
// マージ規則:
//   - runnora.yaml の generate セクションを基準とする (パスはプロジェクトルート基準)
//   - CLI で指定されたフラグは runnora.yaml より優先する (パスはカレントディレクトリ基準)
//   - runnora.yaml がなければ既定値を使う
func buildGenerateOptions(
	lp *loadedProject, openAPIPath, outDir, tagsStr, operationIDsStr,
	mode, caseFormat, caseStyle, server, runnerName string,
	clean, force, skipDeprecated, emitManifest, emitResponseExample bool,
) (*config.GenerateOptions, error) {
	var gen config.GenerateConfig
	if lp.P != nil {
		gen = lp.P.Generate
	}
	fromProject := func(path string) string {
		if path == "" || lp.P == nil {
			return path
		}
		return lp.P.Abs(path)
	}

	opts := &config.GenerateOptions{
		Clean:               clean || gen.CleanGenerated,
		Force:               force,
		SkipDeprecated:      skipDeprecated,
		EmitManifest:        emitManifest || gen.EmitManifest,
		EmitResponseExample: emitResponseExample,
		Server:              server,
	}
	if lp.P != nil {
		opts.ProjectPath = lp.P.Path
	}

	// OpenAPI パス: CLI > runnora.yaml
	opts.OpenAPIPath = firstNonEmpty(openAPIPath, fromProject(gen.OpenAPI))
	if opts.OpenAPIPath == "" {
		return nil, fmt.Errorf("generate: --openapi または runnora.yaml の generate.openapi を指定してください")
	}
	// 出力ディレクトリ: CLI > runnora.yaml > プロジェクトルート (なければ ".")
	defaultOut := "."
	if lp.P != nil {
		defaultOut = lp.P.Root
	}
	opts.OutDir = firstNonEmpty(outDir, fromProject(gen.OutDir), defaultOut)
	opts.Mode = firstNonEmpty(mode, gen.Mode, "shallow")
	opts.CaseFormat = firstNonEmpty(caseFormat, gen.CaseFormat, "json")
	opts.CaseStyle = firstNonEmpty(caseStyle, gen.CaseStyle, "bundled")
	opts.RunnerName = firstNonEmpty(runnerName, gen.RunnerName, "req")

	// tags / operationIDs フィルタ (カンマ区切り → slice)
	if tagsStr != "" {
		opts.Tags = splitTrim(tagsStr)
	}
	if operationIDsStr != "" {
		opts.OperationIDs = splitTrim(operationIDsStr)
	}
	return opts, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// splitTrim はカンマ区切り文字列を trim して slice に変換する。
func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
