package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version 情報はビルド時に -ldflags で注入する。
var (
	Version   = "dev"
	BuildDate = "unknown"
	Commit    = "unknown"
)

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
