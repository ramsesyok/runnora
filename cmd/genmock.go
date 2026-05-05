package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/ramsesyok/oapi2wire/pkg/oapi2wire"
	"github.com/spf13/cobra"
)

// newGenmockCmd は "runnora genmock" サブコマンドを生成する。
//
// OpenAPI と mock case YAML から WireMock の mappings/ と __files/ を生成する
// oapi2wire の公開 API を、runnora の CLI から呼び出せるようにする。
func newGenmockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "genmock",
		Short: "OpenAPI と case YAML から WireMock モックを生成する",
		Long:  "OpenAPI と case YAML から WireMock の mappings/ と __files/ を生成する。",
	}

	cmd.AddCommand(newGenmockInitCmd())
	cmd.AddCommand(newGenmockBuildCmd())
	cmd.AddCommand(newGenmockValidateCmd())

	return cmd
}

func newGenmockInitCmd() *cobra.Command {
	var (
		openAPIPath   string
		outCasesPath  string
		responsesRoot string
		tagsStr       string
		force         bool
		strict        bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "mock case YAML と response stub を生成する",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := oapi2wire.Init(oapi2wire.InitOptions{
				OpenAPIPath:   cleanCLIPath(openAPIPath),
				OutCasesPath:  cleanCLIPath(outCasesPath),
				ResponsesRoot: cleanCLIPath(responsesRoot),
				Force:         force,
				Strict:        strict,
				Tags:          splitTrim(tagsStr),
			})
			if result != nil {
				printGenmockDiagnostics(cmd.ErrOrStderr(), result.Diagnostics)
			}
			if err != nil {
				return fmt.Errorf("genmock init: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "wrote case YAML -> %s\n", result.OutCasesPath)
			fmt.Fprintf(cmd.OutOrStdout(), "generated %d cases, %d response stubs\n", result.GeneratedCases, result.ResponseFilesWritten)
			return nil
		},
	}

	cmd.Flags().StringVar(&openAPIPath, "openapi", "", "OpenAPI ファイルパス (YAML/JSON)")
	cmd.Flags().StringVar(&outCasesPath, "out-cases", "mock-cases.yaml", "生成する mock case YAML のパス")
	cmd.Flags().StringVar(&responsesRoot, "responses-root", "mock-responses", "response stub の出力ディレクトリ")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "生成対象タグ (カンマ区切り): 例 users,orders")
	cmd.Flags().BoolVar(&force, "force", false, "既存ファイルを強制上書きする")
	cmd.Flags().BoolVar(&strict, "strict", false, "警告をエラーとして扱う")
	_ = cmd.MarkFlagRequired("openapi")

	return cmd
}

func newGenmockBuildCmd() *cobra.Command {
	var (
		openAPIPath            string
		casesPath              string
		responsesRoot          string
		outDir                 string
		tagsStr                string
		clean                  bool
		strict                 bool
		failOnMissingOperation bool
		failOnMissingBodyFile  bool
		noAutoFallback         bool
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "WireMock の mappings/ と __files/ を生成する",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := oapi2wire.Build(oapi2wire.BuildOptions{
				OpenAPIPath:            cleanCLIPath(openAPIPath),
				CasesPath:              cleanCLIPath(casesPath),
				ResponsesRoot:          cleanCLIPath(responsesRoot),
				OutDir:                 cleanCLIPath(outDir),
				Clean:                  clean,
				Strict:                 strict,
				FailOnMissingOperation: failOnMissingOperation,
				FailOnMissingBodyFile:  failOnMissingBodyFile,
				NoAutoFallback:         noAutoFallback,
				Tags:                   splitTrim(tagsStr),
			})
			if result != nil {
				printGenmockDiagnostics(cmd.ErrOrStderr(), result.Diagnostics)
			}
			if err != nil {
				return fmt.Errorf("genmock build: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "build complete -> %s\n", result.OutDir)
			fmt.Fprintf(cmd.OutOrStdout(), "generated %d mappings, %d fallbacks\n", result.MappingsWritten, result.FallbacksWritten)
			return nil
		},
	}

	cmd.Flags().StringVar(&openAPIPath, "openapi", "", "OpenAPI ファイルパス (YAML/JSON)")
	cmd.Flags().StringVar(&casesPath, "cases", "mock-cases.yaml", "mock case YAML のパス")
	cmd.Flags().StringVar(&responsesRoot, "responses-root", "mock-responses", "response JSON のルートディレクトリ")
	cmd.Flags().StringVar(&outDir, "out", "wiremock-out", "WireMock 出力ディレクトリ")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "生成対象タグ (カンマ区切り): 例 users,orders")
	cmd.Flags().BoolVar(&clean, "clean", false, "生成前に WireMock 出力ディレクトリを掃除する")
	cmd.Flags().BoolVar(&strict, "strict", false, "警告をエラーとして扱う")
	cmd.Flags().BoolVar(&failOnMissingOperation, "fail-on-missing-operation", false, "case の operationId が OpenAPI に存在しない場合エラーにする")
	cmd.Flags().BoolVar(&failOnMissingBodyFile, "fail-on-missing-body-file", false, "response bodyFile の実ファイルが存在しない場合エラーにする")
	cmd.Flags().BoolVar(&noAutoFallback, "no-auto-fallback", false, "OpenAPI operation の自動 fallback mapping を生成しない")
	_ = cmd.MarkFlagRequired("openapi")

	return cmd
}

func newGenmockValidateCmd() *cobra.Command {
	var (
		openAPIPath            string
		casesPath              string
		responsesRoot          string
		tagsStr                string
		strict                 bool
		failOnMissingOperation bool
		failOnMissingBodyFile  bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "OpenAPI と mock case YAML の整合性を検証する",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := oapi2wire.Validate(oapi2wire.ValidateOptions{
				OpenAPIPath:            cleanCLIPath(openAPIPath),
				CasesPath:              cleanCLIPath(casesPath),
				ResponsesRoot:          cleanCLIPath(responsesRoot),
				Strict:                 strict,
				FailOnMissingOperation: failOnMissingOperation,
				FailOnMissingBodyFile:  failOnMissingBodyFile,
				Tags:                   splitTrim(tagsStr),
			})
			if result != nil {
				printGenmockDiagnostics(cmd.ErrOrStderr(), result.Diagnostics)
			}
			if err != nil {
				return fmt.Errorf("genmock validate: %w", err)
			}
			if result == nil || len(result.Diagnostics) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "OK: no issues found")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&openAPIPath, "openapi", "", "OpenAPI ファイルパス (YAML/JSON)")
	cmd.Flags().StringVar(&casesPath, "cases", "mock-cases.yaml", "mock case YAML のパス")
	cmd.Flags().StringVar(&responsesRoot, "responses-root", "mock-responses", "response JSON のルートディレクトリ")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "検証対象タグ (カンマ区切り): 例 users,orders")
	cmd.Flags().BoolVar(&strict, "strict", false, "警告をエラーとして扱う")
	cmd.Flags().BoolVar(&failOnMissingOperation, "fail-on-missing-operation", false, "case の operationId が OpenAPI に存在しない場合エラーにする")
	cmd.Flags().BoolVar(&failOnMissingBodyFile, "fail-on-missing-body-file", false, "response bodyFile の実ファイルが存在しない場合エラーにする")
	_ = cmd.MarkFlagRequired("openapi")

	return cmd
}

func cleanCLIPath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func printGenmockDiagnostics(w io.Writer, diagnostics []oapi2wire.Diagnostic) {
	for _, d := range diagnostics {
		fmt.Fprintln(w, d.String())
	}
}
