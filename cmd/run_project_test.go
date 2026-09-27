package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
	"github.com/ramsesyok/runnora/internal/app"
)

// projectFixture は runnora.yaml と runbook を持つテスト用プロジェクト。
type projectFixture struct {
	t    *testing.T
	root string
}

func newProjectFixture(t *testing.T, projectYAML string) *projectFixture {
	t.Helper()
	f := &projectFixture{t: t, root: t.TempDir()}
	f.write("runnora.yaml", projectYAML)
	return f
}

func (f *projectFixture) write(rel, content string) string {
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

// execute は runnora を実行し、stdout / stderr / 終了コードを返す。
func (f *projectFixture) execute(args ...string) (string, string, int) {
	f.t.Helper()
	root := cmd.NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	code := 0
	if err != nil {
		code = -1
		var appErr *app.AppError
		if errors.As(err, &appErr) {
			code = appErr.ExitCode
		}
		stderr.WriteString(err.Error())
	}
	return stdout.String(), stderr.String(), code
}

// isolateEnv は runner がプロセスの環境変数に設定する変数を、テスト終了時に元へ戻す。
// 変数はテスト開始時点で未設定にする。
func isolateEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		t.Setenv(n, "")
		os.Unsetenv(n)
	}
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const scenarioBody = `# ${ONLY_IN_COMMENT} はコメントなので未定義でもエラーにしない
runners:
  req:
    endpoint: ${API_URL}
steps:
  hello:
    req:
      %PATH%:
        get:
          body: null
    test: steps.hello.res.status == 200
`

func scenarioRunbook(id, expect, path string, labels ...string) string {
	block := "desc: " + id + "\nrunnora:\n  id: " + id + "\n"
	if expect != "" {
		block += "  expect: " + expect + "\n"
	}
	if len(labels) > 0 {
		block += "labels: [" + strings.Join(labels, ", ") + "]\n"
	}
	return block + strings.ReplaceAll(scenarioBody, "%PATH%", path)
}

func TestRun_SuiteWithExpectationsAndEnvVars(t *testing.T) {
	isolateEnv(t, "API_URL")
	srv := okServer(t)
	f := newProjectFixture(t, `version: 2
project:
  name: sample
defaults:
  env: unit
environments:
  unit:
    description: 単体
    vars:
      API_URL: `+srv.URL+`
suites:
  scenarios:
    select:
      paths: [runbooks/**/*.yml]
      labels: [scenario]
  picked:
    select:
      paths: [runbooks/**/*.yml]
      ids: [S-2, S-1]
`)
	f.write("runbooks/s1.yml", scenarioRunbook("S-1", "", "/ok", "scenario"))
	f.write("runbooks/s2.yml", scenarioRunbook("S-2", "fail", "/fail", "scenario"))
	f.write("runbooks/other.yml", scenarioRunbook("S-3", "", "/ok"))
	f.write("runbooks/part.template.yml", "desc: part\nsteps:\n  s:\n    test: false\n")

	stdout, stderr, code := f.execute("run", "--project", filepath.Join(f.root, "runnora.yaml"), "--suite", "scenarios", "--report-format", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var report struct {
		Project string `json:"project"`
		Suite   string `json:"suite"`
		Env     struct {
			Name string `json:"name"`
		} `json:"env"`
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Results []struct {
			ID     string `json:"id"`
			Expect string `json:"expect"`
			Actual string `json:"actual"`
			Passed bool   `json:"passed"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if report.Project != "sample" || report.Suite != "scenarios" || report.Env.Name != "unit" || report.Total != 2 || report.Passed != 2 {
		t.Fatalf("report = %+v", report)
	}
	if r := report.Results[1]; r.ID != "S-2" || r.Expect != "fail" || r.Actual != "fail" || !r.Passed {
		t.Errorf("expected failure not reported as passed: %+v", r)
	}

	// ids を指定したスイートは ids の順に実行する
	stdout, stderr, code = f.execute("run", "--project", filepath.Join(f.root, "runnora.yaml"), "--suite", "picked", "--report-format", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.Index(stdout, `"S-2"`) > strings.Index(stdout, `"S-1"`) {
		t.Errorf("ids order not kept:\n%s", stdout)
	}
}

func TestRun_VariablePrecedenceAndOverrideNotice(t *testing.T) {
	srv := okServer(t)
	isolateEnv(t, "API_URL")
	f := newProjectFixture(t, `version: 2
environments:
  unit:
    vars:
      API_URL: http://127.0.0.1:1
`)
	book := f.write("runbooks/s1.yml", scenarioRunbook("S-1", "", "/ok"))
	projectFile := filepath.Join(f.root, "runnora.yaml")

	// runnora.yaml の値 (到達できない) のままだと失敗する
	if _, _, code := f.execute("run", "--project", projectFile, book); code != 1 {
		t.Fatalf("expected failure with unreachable yaml value, got exit %d", code)
	}
	// --var が優先される
	if _, stderr, code := f.execute("run", "--project", projectFile, "--var", "API_URL="+srv.URL, book); code != 0 || !strings.Contains(stderr, "env override: API_URL") {
		t.Fatalf("--var: exit %d stderr=%s", code, stderr)
	}
	// OS の環境変数が runnora.yaml より優先され、上書きしたことを表示する
	os.Setenv("API_URL", srv.URL)
	if _, stderr, code := f.execute("run", "--project", projectFile, book); code != 0 || !strings.Contains(stderr, "env override: API_URL") {
		t.Fatalf("OS env: exit %d stderr=%s", code, stderr)
	}
}

func TestRun_ArgumentErrors(t *testing.T) {
	isolateEnv(t, "API_URL")
	f := newProjectFixture(t, "version: 2\nenvironments:\n  unit: {}\nsuites:\n  s:\n    select:\n      paths: [runbooks/*.yml]\n")
	book := f.write("runbooks/s1.yml", scenarioRunbook("S-1", "", "/ok"))
	projectFile := filepath.Join(f.root, "runnora.yaml")
	legacy := newProjectFixture(t, "app:\n  name: old\nhooks:\n  common:\n    before: []\n")

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{name: "--config is removed", args: []string{"run", "--config", "config.yaml", book}, wantMsg: "--config は廃止"},
		{name: "--before-sql is removed", args: []string{"run", "--project", projectFile, "--before-sql", "a.sql", book}, wantMsg: "--before-sql は廃止"},
		{name: "--after-sql is removed", args: []string{"run", "--project", projectFile, "--after-sql", "a.sql", book}, wantMsg: "runnora: ブロック"},
		{name: "legacy runnora.yaml", args: []string{"run", "--project", filepath.Join(legacy.root, "runnora.yaml"), book}, wantMsg: "runnora-migrate"},
		{name: "suite and args together", args: []string{"run", "--project", projectFile, "--suite", "s", book}, wantMsg: "同時に指定できません"},
		{name: "unknown env", args: []string{"run", "--project", projectFile, "--env", "dev", book}, wantMsg: "環境 \"dev\""},
		{name: "undefined variable in runbook", args: []string{"run", "--project", projectFile, book}, wantMsg: "API_URL"},
		{name: "bad --var", args: []string{"run", "--project", projectFile, "--var", "NOEQUALS", book}, wantMsg: "NAME=VALUE"},
		{name: "missing runbook", args: []string{"run", "--project", projectFile, filepath.Join(f.root, "missing.yml")}, wantMsg: "見つかりません"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := f.execute(tt.args...)
			if code != 2 || !strings.Contains(stderr, tt.wantMsg) {
				t.Fatalf("exit %d stderr=%s, want exit 2 containing %q", code, stderr, tt.wantMsg)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	isolateEnv(t, "API_URL", "CI_ONLY_URL_NOT_SET_IN_TESTS", "UNDEFINED_IN_INCLUDE")
	f := newProjectFixture(t, `version: 2
defaults:
  env: unit
environments:
  unit:
    vars:
      API_URL: http://localhost
    oracle:
      dsn: oracle://x
    hooks:
      before: [sql/common.sql]
  ci:
    vars:
      API_URL: ${CI_ONLY_URL_NOT_SET_IN_TESTS}
suites:
  scenarios:
    select:
      paths: [runbooks/*.yml]
  ci:
    env: ci
    select:
      paths: [runbooks/*.yml]
  broken:
    select:
      paths: [runbooks/*.yml]
      ids: [NOPE]
`)
	f.write("runbooks/s1.yml", scenarioRunbook("S-1", "", "/ok"))
	f.write("runbooks/s2.yml", "desc: s2\nrunnora:\n  id: S-2\n  before: [sql/missing.sql]\n  envs: [staging]\nsteps:\n  inc:\n    include: parts/p.yml\n")
	f.write("runbooks/parts/p.yml", "desc: p\nrunners:\n  req: ${UNDEFINED_IN_INCLUDE}\nsteps:\n  s:\n    test: true\n")
	projectFile := filepath.Join(f.root, "runnora.yaml")

	stdout, _, code := f.execute("validate", "--project", projectFile, "--format", "json")
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, stdout)
	}
	var res struct {
		Env      string `json:"env"`
		Runbooks int    `json:"runbooks"`
		Errors   []struct {
			Where   string `json:"where"`
			Message string `json:"message"`
		} `json:"errors"`
		Warnings []struct {
			Where   string `json:"where"`
			Message string `json:"message"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	all := stdout
	for _, want := range []string{"sql/common.sql", "sql/missing.sql", "NOPE", `staging`} {
		if !strings.Contains(all, want) {
			t.Errorf("validate output should mention %q:\n%s", want, stdout)
		}
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0].Message, "UNDEFINED_IN_INCLUDE") {
		t.Errorf("include warning missing: %+v", res.Warnings)
	}
	// ci 環境に固定したスイートは、選んだ環境 (unit) と違うので変数を確かめない
	if strings.Contains(stdout, "CI_ONLY_URL_NOT_SET_IN_TESTS") {
		t.Errorf("variables of a suite pinned to another env should not be checked:\n%s", stdout)
	}
	if res.Env != "unit" || res.Runbooks != 2 {
		t.Errorf("env=%q runbooks=%d", res.Env, res.Runbooks)
	}

	ok := newProjectFixture(t, "version: 2\nenvironments:\n  unit:\n    vars:\n      API_URL: http://x\nsuites:\n  s:\n    select:\n      paths: [runbooks/*.yml]\n")
	ok.write("runbooks/s1.yml", scenarioRunbook("S-1", "", "/ok"))
	if stdout, _, code := ok.execute("validate", "--project", filepath.Join(ok.root, "runnora.yaml")); code != 0 || !strings.Contains(stdout, "OK: 1 runbook") {
		t.Fatalf("valid project: exit %d\n%s", code, stdout)
	}
}

func TestGenerate_ReadsProjectGenerateSection(t *testing.T) {
	f := newProjectFixture(t, "version: 2\ngenerate:\n  openapi: api/openapi.yaml\n  out_dir: out\n")
	f.write("api/openapi.yaml", `openapi: 3.0.3
info: {title: t, version: '1'}
paths:
  /pets:
    get:
      operationId: listPets
      tags: [pet]
      responses:
        '200': {description: ok}
`)
	if _, stderr, code := f.execute("generate", "--project", filepath.Join(f.root, "runnora.yaml")); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(f.root, "out", "runbooks", "generated", "pet", "get_listPets.template.yml")); err != nil {
		t.Fatalf("paths in generate section should be relative to the project root: %v", err)
	}
	if _, stderr, code := f.execute("generate", "--config", "config.yaml"); code != 2 || !strings.Contains(stderr, "--config は廃止") {
		t.Fatalf("generate --config: exit %d %s", code, stderr)
	}
}
