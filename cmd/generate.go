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
		configPath          string
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
			// 設定ファイルを読み込み、CLI フラグで上書きする
			opts, err := buildGenerateOptions(
				configPath, openAPIPath, outDir, tagsStr, operationIDsStr,
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
	cmd.Flags().StringVar(&configPath, "config", "./config.yaml", "設定ファイルパス")
	cmd.Flags().StringVar(&openAPIPath, "openapi", "", "OpenAPI ファイルパス (YAML/JSON)")
	cmd.Flags().StringVar(&outDir, "out", "", "生成物の出力基底ディレクトリ (デフォルト: config の out_dir または .)")
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
	cmd.Flags().BoolVar(&emitResponseExample, "emit-response-example", false, "レスポンス example を case に含める")

	return cmd
}

// buildGenerateOptions は設定ファイルと CLI フラグをマージして GenerateOptions を返す。
//
// マージ規則 (設計書 §15.3):
//   - config の generate セクションを基準とする
//   - CLI で指定されたフラグは config より優先する
func buildGenerateOptions(
	configPath, openAPIPath, outDir, tagsStr, operationIDsStr,
	mode, caseFormat, caseStyle, server, runnerName string,
	clean, force, skipDeprecated, emitManifest, emitResponseExample bool,
) (*config.GenerateOptions, error) {
	// 設定ファイルを読み込む (存在しなければデフォルト値のみ使う)
	cfg, cfgErr := loadConfigOrDefault(configPath)

	opts := &config.GenerateOptions{
		ConfigPath:          configPath,
		Clean:               clean,
		Force:               force,
		SkipDeprecated:      skipDeprecated,
		EmitManifest:        emitManifest,
		EmitResponseExample: emitResponseExample,
	}

	// OpenAPI パス: CLI > config
	if openAPIPath != "" {
		opts.OpenAPIPath = openAPIPath
	} else if cfgErr == nil {
		opts.OpenAPIPath = cfg.Generate.OpenAPI
	}
	if opts.OpenAPIPath == "" {
		return nil, fmt.Errorf("generate: --openapi または config の generate.openapi を指定してください")
	}

	// 出力ディレクトリ: CLI > config > "."
	if outDir != "" {
		opts.OutDir = outDir
	} else if cfgErr == nil && cfg.Generate.OutDir != "" {
		opts.OutDir = cfg.Generate.OutDir
	} else {
		opts.OutDir = "."
	}

	// mode: CLI > config > "shallow"
	if mode != "" {
		opts.Mode = mode
	} else if cfgErr == nil && cfg.Generate.Mode != "" {
		opts.Mode = cfg.Generate.Mode
	} else {
		opts.Mode = "shallow"
	}

	// case-format: CLI > config > "json"
	if caseFormat != "" {
		opts.CaseFormat = caseFormat
	} else if cfgErr == nil && cfg.Generate.CaseFormat != "" {
		opts.CaseFormat = cfg.Generate.CaseFormat
	} else {
		opts.CaseFormat = "json"
	}

	// case-style: CLI > config > "bundled"
	if caseStyle != "" {
		opts.CaseStyle = caseStyle
	} else if cfgErr == nil && cfg.Generate.CaseStyle != "" {
		opts.CaseStyle = cfg.Generate.CaseStyle
	} else {
		opts.CaseStyle = "bundled"
	}

	// runner-name: CLI > config > "req"
	if runnerName != "" {
		opts.RunnerName = runnerName
	} else if cfgErr == nil && cfg.Generate.RunnerName != "" {
		opts.RunnerName = cfg.Generate.RunnerName
	} else {
		opts.RunnerName = "req"
	}

	// emit-manifest: CLI OR config
	if cfgErr == nil && cfg.Generate.EmitManifest {
		opts.EmitManifest = true
	}

	// clean: CLI OR config
	if cfgErr == nil && cfg.Generate.CleanGenerated {
		opts.Clean = true
	}

	// server
	opts.Server = server

	// tags フィルタ (カンマ区切り → slice)
	if tagsStr != "" {
		opts.Tags = splitTrim(tagsStr)
	}

	// operationIDs フィルタ
	if operationIDsStr != "" {
		opts.OperationIDs = splitTrim(operationIDsStr)
	}

	// GenerateOptions を generate パッケージの型に変換して返す
	return opts, nil
}

// loadConfigOrDefault は設定ファイルを読み込む。
// ファイルが存在しない場合はデフォルト値の Config を返す。
func loadConfigOrDefault(path string) (*config.Config, error) {
	// internal/config の Load 関数を再利用する
	// (存在しなければエラーを返すが呼び出し元で無視する)
	return config.Load(path)
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
