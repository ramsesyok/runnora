package hook_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/hook"
	"github.com/ramsesyok/runnora/internal/oracle"
)

// recordingExecutor は呼び出し順を記録する oracle.Executor のテストダブル。
type recordingExecutor struct {
	calls      []string
	onExecFile func(path string) error
}

func (r *recordingExecutor) ExecFile(_ context.Context, path string) error {
	r.calls = append(r.calls, path)
	if r.onExecFile != nil {
		return r.onExecFile(path)
	}
	return nil
}

func (r *recordingExecutor) ExecText(_ context.Context, _ string) error { return nil }
func (r *recordingExecutor) Close() error                                { return nil }

var _ oracle.Executor = (*recordingExecutor)(nil)

func TestRunBefore_ExecutesFilesInCorrectOrder(t *testing.T) {
	rec := &recordingExecutor{}
	files := []string{"common.sql", "cli.sql"}

	if err := hook.RunBefore(context.Background(), rec, files); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(rec.calls))
	}
	if rec.calls[0] != "common.sql" {
		t.Errorf("first call: got %q, want common.sql", rec.calls[0])
	}
	if rec.calls[1] != "cli.sql" {
		t.Errorf("second call: got %q, want cli.sql", rec.calls[1])
	}
}

func TestRunBefore_StopsOnFirstError(t *testing.T) {
	callCount := 0
	rec := &recordingExecutor{
		onExecFile: func(_ string) error {
			callCount++
			return errors.New("ORA-00900: invalid SQL")
		},
	}

	err := hook.RunBefore(context.Background(), rec, []string{"a.sql", "b.sql"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call before stop, got %d", callCount)
	}
}

func TestRunBefore_ErrorIncludesFilePath(t *testing.T) {
	rec := &recordingExecutor{
		onExecFile: func(_ string) error {
			return errors.New("ORA-00900")
		},
	}

	err := hook.RunBefore(context.Background(), rec, []string{"bad.sql"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad.sql") {
		t.Errorf("error should contain file path 'bad.sql': %v", err)
	}
}

func TestRunAfter_ExecutesFilesInGivenOrder(t *testing.T) {
	rec := &recordingExecutor{}
	// BuildAfterFiles が CLI → common の順で組み立てるため、
	// RunAfter はその順序をそのまま実行することを確認する。
	files := []string{"cli_after.sql", "common_after.sql"}

	if err := hook.RunAfter(context.Background(), rec, files); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(rec.calls))
	}
	if rec.calls[0] != "cli_after.sql" {
		t.Errorf("first call: got %q, want cli_after.sql", rec.calls[0])
	}
	if rec.calls[1] != "common_after.sql" {
		t.Errorf("second call: got %q, want common_after.sql", rec.calls[1])
	}
}

func TestRunAfter_ErrorIncludesFilePath(t *testing.T) {
	rec := &recordingExecutor{
		onExecFile: func(_ string) error {
			return errors.New("cleanup failed")
		},
	}

	err := hook.RunAfter(context.Background(), rec, []string{"cleanup.sql"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cleanup.sql") {
		t.Errorf("error should contain file path 'cleanup.sql': %v", err)
	}
}
