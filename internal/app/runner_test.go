package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/config"
	"github.com/ramsesyok/runnora/internal/oracle"
	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/scenario"
)

// recordingExecutor は実行した SQL ファイル名を記録する oracle.Executor。
// failOn に含まれるファイル名 (basename) の実行はエラーにする。
type recordingExecutor struct {
	calls  *[]string
	failOn map[string]bool
}

func (e *recordingExecutor) ExecFile(_ context.Context, path string) error {
	name := filepath.Base(path)
	*e.calls = append(*e.calls, name)
	if e.failOn[name] {
		return fmt.Errorf("ORA-20001: %s failed", name)
	}
	return nil
}
func (e *recordingExecutor) ExecText(_ context.Context, _ string) error { return nil }
func (e *recordingExecutor) Close() error                               { return nil }

var _ oracle.Executor = (*recordingExecutor)(nil)

func recordingFactory(calls *[]string, failOn ...string) app.ExecutorFactory {
	fail := map[string]bool{}
	for _, f := range failOn {
		fail[f] = true
	}
	return func(_ *config.OracleConfig) (oracle.Executor, error) {
		return &recordingExecutor{calls: calls, failOn: fail}, nil
	}
}

func failingFactory(_ *config.OracleConfig) (oracle.Executor, error) {
	return nil, errors.New("connection refused")
}

// testSetenv は t.Setenv を使う setenv。テスト終了時に元に戻る。
func testSetenv(t *testing.T) app.RunnerOption {
	return app.WithSetenv(func(k, v string) error {
		t.Setenv(k, v)
		return nil
	})
}

// fixture はテスト用のプロジェクトルートを作る。
type fixture struct {
	t    *testing.T
	root string
	srv  *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	f := &fixture{t: t, root: t.TempDir(), srv: srv}
	for _, name := range []string{"env_before.sql", "env_after.sql", "sc_before.sql", "sc_after.sql", "boom.sql"} {
		f.write("sql/"+name, "BEGIN NULL; END;")
	}
	return f
}

func (f *fixture) write(rel, content string) string {
	f.t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// runbook は HTTP を 1 回呼ぶ runbook を作って読み込む。path が /fail なら 500 で失敗する。
func (f *fixture) runbook(rel, block, path string) *scenario.Runbook {
	f.t.Helper()
	content := block + fmt.Sprintf(`desc: %s
runners:
  req:
    endpoint: %s
steps:
  hello:
    req:
      %s:
        get:
          body: null
    test: steps.hello.res.status == 200
`, rel, f.srv.URL, path)
	p := f.write(rel, content)
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	return rb
}

func (f *fixture) env(before, after []string) *project.Resolved {
	abs := func(names []string) []string {
		var out []string
		for _, n := range names {
			out = append(out, filepath.Join(f.root, "sql", n))
		}
		return out
	}
	return &project.Resolved{
		Name: "unit", HasOracle: true, Oracle: config.OracleConfig{DSN: "oracle://x"},
		Before: abs(before), After: abs(after),
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var appErr *app.AppError
	if errors.As(err, &appErr) {
		return appErr.ExitCode
	}
	return -1
}

func TestRunner_NoRunbooks_ReturnsExitCode2(t *testing.T) {
	r := app.NewRunner(testSetenv(t))
	if _, err := r.Run(context.Background(), &app.Plan{}); exitCode(err) != 2 {
		t.Fatalf("exit code = %d (%v), want 2", exitCode(err), err)
	}
}

// 5.4 の表: expect と実際の結果の組み合わせごとに、合否と終了コードを確かめる。
func TestRunner_ExpectAndExitCode(t *testing.T) {
	tests := []struct {
		name       string
		expect     string
		path       string
		failOn     []string
		wantActual string
		wantPassed bool
		wantExit   int
	}{
		{name: "pass/pass", expect: "pass", path: "/ok", wantActual: "pass", wantPassed: true, wantExit: 0},
		{name: "pass/fail", expect: "pass", path: "/fail", wantActual: "fail", wantPassed: false, wantExit: 1},
		{name: "pass/hookFail", expect: "pass", path: "/ok", failOn: []string{"sc_after.sql"}, wantActual: "hookFail", wantPassed: false, wantExit: 4},
		{name: "fail/fail", expect: "fail", path: "/fail", wantActual: "fail", wantPassed: true, wantExit: 0},
		{name: "fail/pass", expect: "fail", path: "/ok", wantActual: "pass", wantPassed: false, wantExit: 1},
		{name: "fail/hookFail", expect: "fail", path: "/ok", failOn: []string{"sc_before.sql"}, wantActual: "hookFail", wantPassed: false, wantExit: 4},
		{name: "hookFail/hookFail", expect: "hookFail", path: "/ok", failOn: []string{"sc_after.sql"}, wantActual: "hookFail", wantPassed: true, wantExit: 0},
		{name: "hookFail/pass", expect: "hookFail", path: "/ok", wantActual: "pass", wantPassed: false, wantExit: 1},
		{name: "hookFail/fail", expect: "hookFail", path: "/fail", wantActual: "fail", wantPassed: false, wantExit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			block := "runnora:\n  id: CASE-1\n  before: [sql/sc_before.sql]\n  after: [sql/sc_after.sql]\n  expect: " + tt.expect + "\n"
			rb := f.runbook("runbooks/case.yml", block, tt.path)
			var calls []string
			r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(recordingFactory(&calls, tt.failOn...)))

			report, err := r.Run(context.Background(), &app.Plan{Root: f.root, Env: f.env(nil, nil), Runbooks: []*scenario.Runbook{rb}})
			if exitCode(err) != tt.wantExit {
				t.Fatalf("exit code = %d (%v), want %d", exitCode(err), err, tt.wantExit)
			}
			if report == nil || len(report.Results) != 1 {
				t.Fatalf("report = %+v", report)
			}
			got := report.Results[0]
			if got.ID != "CASE-1" || got.Expect != tt.expect || got.Actual != tt.wantActual || got.Passed != tt.wantPassed {
				t.Errorf("result = %+v, want actual=%s passed=%v", got, tt.wantActual, tt.wantPassed)
			}
		})
	}
}

func TestRunner_HookOrder(t *testing.T) {
	tests := []struct {
		name      string
		envBefore []string
		envAfter  []string
		block     string
		want      []string
	}{
		{
			name:      "env and scenario hooks",
			envBefore: []string{"env_before.sql"}, envAfter: []string{"env_after.sql"},
			block: "runnora:\n  id: A\n  before: [sql/sc_before.sql]\n  after: [sql/sc_after.sql]\n",
			want:  []string{"env_before.sql", "sc_before.sql", "sc_after.sql", "env_after.sql"},
		},
		{
			name:      "env hooks only",
			envBefore: []string{"env_before.sql"}, envAfter: []string{"env_after.sql"},
			block: "runnora:\n  id: A\n",
			want:  []string{"env_before.sql", "env_after.sql"},
		},
		{
			name:  "scenario hooks only",
			block: "runnora:\n  id: A\n  before: [sql/sc_before.sql]\n",
			want:  []string{"sc_before.sql"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			rb := f.runbook("runbooks/a.yml", tt.block, "/ok")
			var calls []string
			r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(recordingFactory(&calls)))
			if _, err := r.Run(context.Background(), &app.Plan{Root: f.root, Env: f.env(tt.envBefore, tt.envAfter), Runbooks: []*scenario.Runbook{rb}}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tt.want) {
				t.Errorf("calls = %v, want %v", calls, tt.want)
			}
		})
	}
}

func TestRunner_HooksArePerRunbook(t *testing.T) {
	f := newFixture(t)
	a := f.runbook("runbooks/a.yml", "runnora:\n  id: A\n  before: [sql/sc_before.sql]\n", "/ok")
	b := f.runbook("runbooks/b.yml", "runnora:\n  id: B\n  after: [sql/sc_after.sql]\n", "/ok")
	var calls []string
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(recordingFactory(&calls)))
	if _, err := r.Run(context.Background(), &app.Plan{Root: f.root, Env: f.env([]string{"env_before.sql"}, nil), Runbooks: []*scenario.Runbook{a, b}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"env_before.sql", "sc_before.sql", "env_before.sql", "sc_after.sql"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestRunner_IncludedBlockIsIgnored(t *testing.T) {
	f := newFixture(t)
	f.write("runbooks/child.yml", "desc: child\nrunnora:\n  id: CHILD\n  before: [sql/boom.sql]\nsteps:\n  s:\n    test: true\n")
	p := f.write("runbooks/parent.yml", "desc: parent\nrunnora:\n  id: PARENT\n  before: [sql/sc_before.sql]\nsteps:\n  inc:\n    include: child.yml\n")
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(recordingFactory(&calls, "boom.sql")))
	if _, err := r.Run(context.Background(), &app.Plan{Root: f.root, Env: f.env(nil, nil), Runbooks: []*scenario.Runbook{rb}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"sc_before.sql"}) {
		t.Errorf("calls = %v, want only the parent's hook", calls)
	}
}

func TestRunner_NoHooks_SkipsDBConnection(t *testing.T) {
	f := newFixture(t)
	rb := f.runbook("runbooks/a.yml", "", "/ok")
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(failingFactory))
	report, err := r.Run(context.Background(), &app.Plan{Root: f.root, Runbooks: []*scenario.Runbook{rb}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Passed != 1 || report.Results[0].ID != "runbooks/a" {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunner_SetupErrors(t *testing.T) {
	tests := []struct {
		name     string
		block    string
		env      func(f *fixture) *project.Resolved
		factory  app.ExecutorFactory
		wantExit int
		wantMsg  string
	}{
		{
			name:     "hooks without oracle settings",
			block:    "runnora:\n  id: A\n  before: [sql/sc_before.sql]\n",
			env:      func(f *fixture) *project.Resolved { return &project.Resolved{Name: "mock"} },
			wantExit: 2, wantMsg: "oracle が設定されていません",
		},
		{
			name:     "connection failure",
			block:    "runnora:\n  id: A\n  before: [sql/sc_before.sql]\n",
			env:      func(f *fixture) *project.Resolved { return f.env(nil, nil) },
			factory:  failingFactory,
			wantExit: 3, wantMsg: "connection refused",
		},
		{
			name:     "missing SQL file",
			block:    "runnora:\n  id: A\n  before: [sql/missing.sql]\n",
			env:      func(f *fixture) *project.Resolved { return f.env(nil, nil) },
			wantExit: 4, wantMsg: "missing.sql",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			rb := f.runbook("runbooks/a.yml", tt.block, "/ok")
			var calls []string
			factory := tt.factory
			if factory == nil {
				factory = recordingFactory(&calls)
			}
			r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(factory))
			_, err := r.Run(context.Background(), &app.Plan{Root: f.root, Env: tt.env(f), Runbooks: []*scenario.Runbook{rb}})
			if exitCode(err) != tt.wantExit || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %v (exit %d), want exit %d containing %q", err, exitCode(err), tt.wantExit, tt.wantMsg)
			}
		})
	}
}

func TestRunner_EnvsFilterSkipsRunbook(t *testing.T) {
	f := newFixture(t)
	onlyUnit := f.runbook("runbooks/a.yml", "runnora:\n  id: A\n  envs: [unit]\n", "/ok")
	onlyInt := f.runbook("runbooks/b.yml", "runnora:\n  id: B\n  envs: [integration]\n  before: [sql/boom.sql]\n", "/fail")
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(failingFactory))
	report, err := r.Run(context.Background(), &app.Plan{Root: f.root, Env: &project.Resolved{Name: "unit"}, Runbooks: []*scenario.Runbook{onlyUnit, onlyInt}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Total != 2 || report.Passed != 1 || report.Skipped != 1 || report.Failed != 0 {
		t.Fatalf("report = %+v", report)
	}
	// 結果は実行順 (引数の順) に並ぶ
	if report.Results[0].ID != "A" || report.Results[1].ID != "B" || report.Results[1].Actual != "skipped" {
		t.Errorf("results = %+v", report.Results)
	}
	if report.Results[0].Path != "runbooks/a.yml" {
		t.Errorf("path should be relative to the root: %q", report.Results[0].Path)
	}
}

func TestRunner_EnvVarsReachRunbook(t *testing.T) {
	f := newFixture(t)
	p := f.write("runbooks/vars.yml", `desc: vars
runnora:
  id: VARS
runners:
  req:
    endpoint: ${RUNNORA_TEST_API_URL}
steps:
  hello:
    req:
      /ok:
        get:
          body: null
    test: steps.hello.res.status == 200
`)
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		t.Fatal(err)
	}
	env := &project.Resolved{Name: "unit", Description: "desc", Vars: map[string]string{"RUNNORA_TEST_API_URL": f.srv.URL}, Overrides: []string{"X"},
		Backends: map[string]project.Backend{"calc": {Mode: "stub"}}}
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(failingFactory))
	report, err := r.Run(context.Background(), &app.Plan{Root: f.root, ProjectName: "proj", Suite: "s", Env: env, Trace: false, Runbooks: []*scenario.Runbook{rb}})
	if err != nil {
		t.Fatalf("unexpected error: %v (%+v)", err, report)
	}
	if report.Project != "proj" || report.Suite != "s" || report.Env == nil || report.Env.Name != "unit" ||
		!reflect.DeepEqual(report.Env.Overrides, []string{"X"}) || report.Backends["calc"].Mode != "stub" {
		t.Errorf("report header = %+v env=%+v", report, report.Env)
	}
}

func TestRunner_FailFastStopsAfterFirstMismatch(t *testing.T) {
	f := newFixture(t)
	a := f.runbook("runbooks/a.yml", "runnora:\n  id: A\n", "/fail")
	b := f.runbook("runbooks/b.yml", "runnora:\n  id: B\n", "/ok")
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(failingFactory))
	report, err := r.Run(context.Background(), &app.Plan{Root: f.root, FailFast: true, Runbooks: []*scenario.Runbook{a, b}})
	if exitCode(err) != 1 || len(report.Results) != 1 {
		t.Fatalf("err=%v report=%+v", err, report)
	}
}
