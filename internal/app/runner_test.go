package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/oracle"
)

// noopExecutor は何もしない oracle.Executor。
type noopExecutor struct{}

func (n *noopExecutor) ExecFile(_ context.Context, _ string) error { return nil }
func (n *noopExecutor) ExecText(_ context.Context, _ string) error { return nil }
func (n *noopExecutor) Close() error                               { return nil }

var _ oracle.Executor = (*noopExecutor)(nil)

func noopFactory(_ *config.DBConfig) (oracle.Executor, error) {
	return &noopExecutor{}, nil
}

func failingFactory(_ *config.DBConfig) (oracle.Executor, error) {
	return nil, errors.New("connection refused")
}

// validConfigPath は hooks なしの最小限の config.yaml を作成して返す。
func validConfigPath(t *testing.T) string {
	t.Helper()
	content := `app: {name: test}`
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatalf("setup config: %v", err)
	}
	return f
}

// configWithBeforeHook は before フックを持つ config.yaml を作成して返す。
func configWithBeforeHook(t *testing.T, hookFile string) string {
	t.Helper()
	content := fmt.Sprintf(`
hooks:
  common:
    before:
      - %q
`, hookFile)
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatalf("setup config: %v", err)
	}
	return f
}

// writeHTTPRunbook は httptest.Server の URL にアクセスする runbook ファイルを作成して返す。
func writeHTTPRunbook(t *testing.T, serverURL string) string {
	t.Helper()
	content := fmt.Sprintf(`
desc: simple HTTP test
runners:
  req:
    endpoint: %s
steps:
  hello:
    req:
      /hello:
        get:
          body: null
    test: steps.hello.res.status == 200
`, serverURL)
	f := filepath.Join(t.TempDir(), "test.yml")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatalf("write runbook: %v", err)
	}
	return f
}

func TestRunner_Run_BadConfigPath_ReturnsExitCode2(t *testing.T) {
	r := app.NewRunner(app.WithExecutorFactory(noopFactory))
	_, err := r.Run(context.Background(), &config.RunOptions{
		ConfigPath:   "/nonexistent/config.yaml",
		RunbookPaths: []string{"any.yml"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *app.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got: %T %v", err, err)
	}
	if appErr.ExitCode != 2 {
		t.Errorf("expected exit code 2, got %d", appErr.ExitCode)
	}
}

func TestRunner_Run_NoRunbooks_ReturnsExitCode2(t *testing.T) {
	r := app.NewRunner(app.WithExecutorFactory(noopFactory))
	_, err := r.Run(context.Background(), &config.RunOptions{
		ConfigPath:   validConfigPath(t),
		RunbookPaths: []string{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *app.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got: %T %v", err, err)
	}
	if appErr.ExitCode != 2 {
		t.Errorf("expected exit code 2, got %d", appErr.ExitCode)
	}
}

func TestRunner_Run_DBConnectionFail_ReturnsExitCode3(t *testing.T) {
	r := app.NewRunner(app.WithExecutorFactory(failingFactory))
	_, err := r.Run(context.Background(), &config.RunOptions{
		ConfigPath:   validConfigPath(t),
		RunbookPaths: []string{"any.yml"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *app.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got: %T %v", err, err)
	}
	if appErr.ExitCode != 3 {
		t.Errorf("expected exit code 3, got %d", appErr.ExitCode)
	}
}

func TestRunner_Run_BeforeHookFail_ReturnsExitCode4(t *testing.T) {
	failingExec := &failingExecutor{}
	r := app.NewRunner(app.WithExecutorFactory(func(_ *config.DBConfig) (oracle.Executor, error) {
		return failingExec, nil
	}))

	// before フックを持つ config を用意する
	hookFile := filepath.Join(t.TempDir(), "hook.sql")
	os.WriteFile(hookFile, []byte("BEGIN NULL; END;"), 0o600)
	cfgPath := configWithBeforeHook(t, hookFile)

	// runn に渡す runbook は存在しなくてもよい（before 失敗で止まる）
	runbookPath := filepath.Join(t.TempDir(), "dummy.yml")
	os.WriteFile(runbookPath, []byte(`desc: dummy
steps:
  noop:
    exec:
      command: echo ok
`), 0o600)

	_, err := r.Run(context.Background(), &config.RunOptions{
		ConfigPath:   cfgPath,
		RunbookPaths: []string{runbookPath},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *app.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got: %T %v", err, err)
	}
	if appErr.ExitCode != 4 {
		t.Errorf("expected exit code 4, got %d: %v", appErr.ExitCode, err)
	}
}

func TestRunner_Run_AllRunbooksPass_ReportShowsCorrectCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	runbook := writeHTTPRunbook(t, srv.URL)

	r := app.NewRunner(app.WithExecutorFactory(noopFactory))
	report, err := r.Run(context.Background(), &config.RunOptions{
		ConfigPath:   validConfigPath(t),
		RunbookPaths: []string{runbook},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected report, got nil")
	}
	if report.Total != 1 {
		t.Errorf("got Total=%d, want 1", report.Total)
	}
	if report.Passed != 1 {
		t.Errorf("got Passed=%d, want 1", report.Passed)
	}
	if report.Failed != 0 {
		t.Errorf("got Failed=%d, want 0", report.Failed)
	}
}

func TestRunner_Run_RunbookFails_ReportShowsFailureAndExitCode1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	runbook := writeHTTPRunbook(t, srv.URL)

	r := app.NewRunner(app.WithExecutorFactory(noopFactory))
	_, err := r.Run(context.Background(), &config.RunOptions{
		ConfigPath:   validConfigPath(t),
		RunbookPaths: []string{runbook},
	})
	if err == nil {
		t.Fatal("expected error for failing runbook, got nil")
	}
	var appErr *app.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got: %T %v", err, err)
	}
	if appErr.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", appErr.ExitCode)
	}
}

// failingExecutor は ExecFile で必ずエラーを返す oracle.Executor。
type failingExecutor struct{}

func (f *failingExecutor) ExecFile(_ context.Context, path string) error {
	return fmt.Errorf("ORA-12345: hook failed: %s", path)
}
func (f *failingExecutor) ExecText(_ context.Context, _ string) error { return nil }
func (f *failingExecutor) Close() error                               { return nil }
