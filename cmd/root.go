package cmd

import (
	"github.com/spf13/cobra"
)

// NewRootCmd はルートコマンドを生成して返す。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "runnora",
		Short: "WebAPI/gRPC シナリオテストツール",
		Long:  "runnora は runn を組み込んだ WebAPI/gRPC シナリオテストツールです。",
	}

	root.AddCommand(newRunCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newCoverageCmd())
	root.AddCommand(newLoadtCmd())
	root.AddCommand(newNewCmd())
	root.AddCommand(newRprofCmd())

	return root
}

// Execute はルートコマンドを実行する。
func Execute() error {
	return NewRootCmd().Execute()
}
