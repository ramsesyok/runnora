package hook

import (
	"context"
	"fmt"

	"github.com/ramsesyok/runnora/internal/oracle"
)

// RunBefore は before フックのファイルリストを順番に実行する。
// 最初のエラーで停止し、エラーにはファイルパスを含める。
func RunBefore(ctx context.Context, exec oracle.Executor, files []string) error {
	return runFiles(ctx, exec, "before", files)
}

// RunAfter は after フックのファイルリストを順番に実行する。
// エラーが発生してもエラーにはファイルパスを含める。
func RunAfter(ctx context.Context, exec oracle.Executor, files []string) error {
	return runFiles(ctx, exec, "after", files)
}

func runFiles(ctx context.Context, exec oracle.Executor, phase string, files []string) error {
	for _, f := range files {
		if err := exec.ExecFile(ctx, f); err != nil {
			return fmt.Errorf("hook %s %s: %w", phase, f, err)
		}
	}
	return nil
}
