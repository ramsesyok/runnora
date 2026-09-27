package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/reporter"
	"github.com/ramsesyok/runnora/internal/scenario"
)

// ステップ単位の結果: include 先、loop で include した回、if で飛ばしたステップ、
// 失敗したステップとその後の実行しなかったステップ、前後処理、証跡の対応。
func TestRun_StepResults(t *testing.T) {
	f := newFixture(t)
	f.write("runbooks/part.yml", fmt.Sprintf(`desc: part
runners:
  req: %s
steps:
  call:
    req:
      /part:
        get:
          body: null
`, f.srv.URL))
	p := f.write("runbooks/main.yml", fmt.Sprintf(`desc: main scenario
runnora:
  id: MAIN-1
  before: [sql/sc_before.sql]
runners:
  req: %s
steps:
  first:
    desc: 最初の呼び出し
    req:
      /first:
        get:
          body: null
    test: current.res.status == 200
  inc:
    include:
      path: part.yml
  twice:
    loop:
      count: 2
    include:
      path: part.yml
  skipped:
    if: 'len("x") == 0'
    test: true
  broken:
    req:
      /fail:
        get:
          body: null
    test: current.res.status == 200
  never:
    test: true
`, f.srv.URL))
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(recordingFactory(&calls)))
	evDir := filepath.Join(t.TempDir(), "evidence")
	report, err := r.Run(context.Background(), &app.Plan{
		Root: f.root, Env: f.env([]string{"env_before.sql"}, []string{"env_after.sql"}),
		Runbooks: []*scenario.Runbook{rb}, EvidenceDir: evDir, Version: "v9.9.9",
	})
	if exitCode(err) != 1 {
		t.Fatalf("exit %d (%v), want 1", exitCode(err), err)
	}
	if report.SchemaVersion != reporter.SchemaVersion || report.Runnora != "v9.9.9" || report.StartedAt == "" {
		t.Errorf("report header: %+v", report)
	}
	res := report.Results[0]
	if res.Desc != "main scenario" || res.ElapsedMs < 0 {
		t.Errorf("result: %+v", res)
	}

	var got []string
	for _, s := range res.Steps {
		line := fmt.Sprintf("%d %s %s %s", s.Index, s.Key, s.Runner, s.Result)
		if len(s.Evidence) > 0 {
			line += " " + strings.Join(s.Evidence, ",")
		}
		got = append(got, line)
	}
	want := []string{
		"1 first http success MAIN-1/01-first.json",
		"2 inc include success",
		"2 inc.call http success MAIN-1/02-inc.call.json",
		"3 twice include success",
		"3 twice.call http success MAIN-1/03-twice[0].call.json,MAIN-1/03-twice[1].call.json",
		"4 skipped test skipped",
		"5 broken http failure MAIN-1/05-broken.json",
		"6 never test notRun",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("steps:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if res.Steps[0].Desc != "最初の呼び出し" {
		t.Errorf("desc: %q", res.Steps[0].Desc)
	}
	if !strings.Contains(res.Steps[6].Error, "current.res.status == 200") {
		t.Errorf("failure message: %q", res.Steps[6].Error)
	}

	var hooks []string
	for _, h := range res.Hooks {
		hooks = append(hooks, fmt.Sprintf("%s %s %v", h.Phase, h.File, h.OK))
	}
	wantHooks := []string{"before sql/env_before.sql true", "before sql/sc_before.sql true", "after sql/env_after.sql true"}
	if strings.Join(hooks, "\n") != strings.Join(wantHooks, "\n") {
		t.Errorf("hooks:\n%s", strings.Join(hooks, "\n"))
	}
}

// 前後処理が失敗したら、そのファイルが ok: false とエラーで記録され、後のファイルは載らない。
func TestRun_HookResults(t *testing.T) {
	f := newFixture(t)
	rb := f.runbook("runbooks/a.yml", "runnora:\n  id: A\n  after: [sql/boom.sql]\n  expect: hookFail\n", "/ok")
	var calls []string
	r := app.NewRunner(testSetenv(t), app.WithExecutorFactory(recordingFactory(&calls, "boom.sql")))
	report, err := r.Run(context.Background(), &app.Plan{
		Root: f.root, Env: f.env(nil, []string{"env_after.sql"}), Runbooks: []*scenario.Runbook{rb},
	})
	if err != nil {
		t.Fatalf("expected hookFail should pass: %v", err)
	}
	h := report.Results[0].Hooks
	if len(h) != 1 || h[0].File != "sql/boom.sql" || h[0].OK || !strings.Contains(h[0].Error, "ORA-20001") {
		t.Fatalf("hooks: %+v", h)
	}
}

// 所要時間を、実行全体・runbook・ステップのそれぞれに記録する。
func TestRun_ElapsedTimes(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(slow.Close)
	f := newFixture(t)
	p := f.write("runbooks/slow.yml", fmt.Sprintf("desc: slow\nrunnora:\n  id: SLOW\nrunners:\n  req: %s\nsteps:\n  wait:\n    req:\n      /wait:\n        get:\n          body: null\n", slow.URL))
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := app.NewRunner(testSetenv(t)).Run(context.Background(), &app.Plan{Root: f.root, Runbooks: []*scenario.Runbook{rb}})
	if err != nil {
		t.Fatal(err)
	}
	res := report.Results[0]
	if report.ElapsedMs < 25 || res.ElapsedMs < 25 || len(res.Steps) != 1 || res.Steps[0].ElapsedMs < 25 {
		t.Fatalf("elapsed: report %d, runbook %d, steps %+v", report.ElapsedMs, res.ElapsedMs, res.Steps)
	}
}
