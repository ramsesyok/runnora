package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version 情報はビルド時に -ldflags で注入する (リリースは .goreleaser.yaml)。
//
//	例: go build -ldflags "-X github.com/ramsesyok/runnora/cmd.Version=v1.0.0 \
//	                        -X github.com/ramsesyok/runnora/cmd.Commit=abc1234 \
//	                        -X github.com/ramsesyok/runnora/cmd.BuildDate=2026-03-20"
var (
	Version   = "dev"     // リリースタグ (例: "v1.2.3")
	BuildDate = "unknown" // ビルド日時 (例: "2026-03-20T14:00:00Z")
	Commit    = "unknown" // git commit ハッシュ (例: "abc1234")
)

func init() {
	// "go install ...@v1.2.3" でビルドした場合は、ビルド情報に記録されたモジュールのバージョンを使う
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
}

// newVersionCmd は "runnora version" サブコマンドを生成する。
// バージョン・コミット・ビルド日時を一行で表示する。
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "バージョン情報を表示する",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "runnora %s (commit: %s, built: %s)\n",
				Version, Commit, BuildDate)
		},
	}
}
