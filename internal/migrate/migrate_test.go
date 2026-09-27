package migrate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/scenario"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func changeOf(p *Plan, path string) *Change {
	for i := range p.Changes {
		if p.Changes[i].Path == path {
			return &p.Changes[i]
		}
	}
	return nil
}

func todosContaining(p *Plan, s string) []Todo {
	var out []Todo
	for _, td := range p.Todos {
		if strings.Contains(td.Where+": "+td.Message, s) {
			out = append(out, td)
		}
	}
	return out
}

const legacyConfigYAML = `app:
  name: legacy-app
oracle:
  driver: oracle
  dsn: "oracle://u:p@db:1521/SVC"
  max_open_conns: 5
runn:
  trace: true
hooks:
  common:
    before:
      - "./sql/common/reset.sql"
    after:
      - "./sql/common/verify.sql"
report:
  format: "junit"
generate:
  openapi: openapi.yaml
  emit_manifest: true
`

func TestDiscoverEnvs(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"config.yaml":       "app: {}\n",
		"config.mock.yaml":  "app: {}\n",
		"config-other.yaml": "app: {}\n",
		"configs/x.yaml":    "app: {}\n",
	})
	got, err := DiscoverEnvs(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []EnvSpec{{Name: "default", File: "config.yaml"}, {Name: "mock", File: "config.mock.yaml"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuild_ConfigBecomesProject(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"config.yaml":      legacyConfigYAML,
		"config.mock.yaml": "app:\n  name: mock\noracle:\n  dsn: \"\"\n",
		"runbooks/a.yml":   "desc: a\nsteps:\n  s:\n    test: true\n",
	})
	plan, err := Build(Options{Dir: dir, Envs: []EnvSpec{{Name: "unit", File: "config.yaml"}, {Name: "mock", File: "config.mock.yaml"}}})
	if err != nil {
		t.Fatal(err)
	}
	c := changeOf(plan, "runnora.yaml")
	if c == nil || c.Action != "create" {
		t.Fatalf("runnora.yaml not planned: %+v", plan.Changes)
	}
	p, err := project.Parse([]byte(c.Content))
	if err != nil {
		t.Fatalf("generated runnora.yaml is invalid: %v\n%s", err, c.Content)
	}
	unit := p.Environments["unit"]
	if p.Info.Name != "legacy-app" || p.Defaults.Env != "unit" || unit == nil || p.Environments["mock"] == nil {
		t.Fatalf("project = %+v", p)
	}
	if unit.Vars["ORACLE_DSN"] != "oracle://u:p@db:1521/SVC" || unit.Oracle == nil || unit.Oracle.DSN != "${ORACLE_DSN}" || unit.Oracle.MaxOpenConns != 5 {
		t.Errorf("unit oracle = %+v vars=%v", unit.Oracle, unit.Vars)
	}
	if !reflect.DeepEqual(unit.Hooks.Before, []string{"sql/common/reset.sql"}) || !reflect.DeepEqual(unit.Hooks.After, []string{"sql/common/verify.sql"}) {
		t.Errorf("hooks = %+v", unit.Hooks)
	}
	if p.Environments["mock"].Oracle != nil {
		t.Error("mock env has an empty DSN and should have no oracle section")
	}
	if !p.Runn.Trace || p.Report.Format != "junit" || p.Generate.OpenAPI != "openapi.yaml" || !p.Generate.EmitManifest {
		t.Errorf("runn/report/generate not carried over: %+v %+v %+v", p.Runn, p.Report, p.Generate)
	}
	for _, f := range []string{"config.yaml", "config.mock.yaml"} {
		if c := changeOf(plan, f); c == nil || c.Action != "delete" {
			t.Errorf("%s should be deleted: %+v", f, c)
		}
	}
}

func TestBuild_RunbookEdits(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"config.yaml": legacyConfigYAML,
		"scenarios.yaml": `scenarios:
  - runbook: runbooks/scenarios/lib-004.yml
    before: [./sql/cases/lib004.sql]
  - runbook: runbooks/demo/*.yml
    id: DEMO-1
    after: [sql/cases/break.sql]
    expectExit: 4
`,
		"runbooks/scenarios/lib-004.yml": `desc: LIB-004 貸出上限
# 実行時の注意
labels: [scenario]
runners:
  req:
    endpoint: http://127.0.0.1:18081   # 実 API
    openapi3: ../../openapi.yaml
  db: "oracle://u:p@db:1521/SVC"
  other: https://other.example.com
steps:
  inc:
    include: ./part.yml
`,
		"runbooks/scenarios/part.yml":          "desc: part\nrunners:\n  req: http://127.0.0.1:18081\nsteps:\n  s:\n    test: true\n",
		"runbooks/demo/demo.yml":               "desc: |\n  複数行の\n  説明\nrunners:\n  req:\n    endpoint: ${RUNNORA_BASE_URL}\nsteps:\n  s:\n    test: true\n",
		"runbooks/free.yml":                    "runners:\n  greq:\n    addr: 127.0.0.1:19090\n    tls: false\nsteps:\n  s:\n    test: true\n",
		"runbooks/generated/x/get_x.suite.yml": "desc: gen\nrunners:\n  req: http://gen\nsteps:\n  s:\n    test: true\n",
		"mock-cases.yaml":                      "version: 1\ncases: []\n",
		"scripts/run.ps1":                      "runnora run --config config.yaml --before-sql a.sql x.yml\nrunnora run --scopes run:exec\n",
	})
	plan, err := Build(Options{Dir: dir, ScenariosFile: "scenarios.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	lib := changeOf(plan, "runbooks/scenarios/lib-004.yml")
	if lib == nil {
		t.Fatalf("lib-004 not modified: %+v", plan.Changes)
	}
	wantLib := `desc: LIB-004 貸出上限
runnora:
  id: LIB-004
  before:
    - sql/cases/lib004.sql
# 実行時の注意
labels: [scenario]
runners:
  req:
    endpoint: ${API_URL}   # 実 API
    openapi3: ../../openapi.yaml
  db: ${ORACLE_DSN}
  other: ${OTHER_URL}
steps:
  inc:
    include: ./part.yml
`
	if lib.Content != wantLib {
		t.Errorf("lib-004 content:\n%s\nwant:\n%s", lib.Content, wantLib)
	}

	// include される部品には runnora: ブロックを付けず、値だけ変数にする
	part := changeOf(plan, "runbooks/scenarios/part.yml")
	if part == nil || strings.Contains(part.Content, "runnora:") || !strings.Contains(part.Content, "req: ${API_URL}") {
		t.Errorf("part = %+v", part)
	}

	// 複数行の desc の後、対応表の glob・id・expectExit を使う
	demo := changeOf(plan, "runbooks/demo/demo.yml")
	wantDemo := "desc: |\n  複数行の\n  説明\nrunnora:\n  id: DEMO-1\n  after:\n    - sql/cases/break.sql\n  expect: hookFail\nrunners:\n  req:\n    endpoint: ${RUNNORA_BASE_URL}\nsteps:\n  s:\n    test: true\n"
	if demo == nil || demo.Content != wantDemo {
		t.Errorf("demo content:\n%+v\nwant:\n%s", demo, wantDemo)
	}

	// desc がなければ先頭に追加し、対応表になければ TODO を付ける
	free := changeOf(plan, "runbooks/free.yml")
	if free == nil || !strings.HasPrefix(free.Content, "runnora:\n  # TODO(runnora-migrate)") || !strings.Contains(free.Content, "  id: free\n") || !strings.Contains(free.Content, "addr: ${GRPC_ADDR}") {
		t.Errorf("free = %+v", free)
	}

	if changeOf(plan, "runbooks/generated/x/get_x.suite.yml") != nil || changeOf(plan, "mock-cases.yaml") != nil {
		t.Error("generated runbooks and non-runbook YAML must not change")
	}

	// 取り出した変数は全環境に入り、TODO になる。ORACLE_DSN は config の値と同じなので再利用
	project := changeOf(plan, "runnora.yaml").Content
	for _, want := range []string{`API_URL: "http://127.0.0.1:18081"  # runbook から取り出した値`, `OTHER_URL: "https://other.example.com"`, `GRPC_ADDR: "127.0.0.1:19090"`} {
		if !strings.Contains(project, want) {
			t.Errorf("runnora.yaml should contain %q:\n%s", want, project)
		}
	}
	if strings.Count(project, "ORACLE_DSN:") != 1 {
		t.Errorf("ORACLE_DSN should be defined once:\n%s", project)
	}
	for _, want := range []string{"RUNNORA_BASE_URL がどの環境にも定義されていません", "runbooks/free.yml", "scripts/run.ps1:1: --config", "scripts/run.ps1:1: --before-sql", "scripts/run.ps1:2: --scopes", "runnora generate の出力 (1 ファイル)"} {
		if len(todosContaining(plan, want)) == 0 {
			t.Errorf("TODO %q missing: %+v", want, plan.Todos)
		}
	}
	if len(todosContaining(plan, "lib-004.yml")) != 0 {
		t.Errorf("mapped scenario should not get a TODO: %+v", todosContaining(plan, "lib-004.yml"))
	}
}

func TestApply_ResultValidatesAndIsIdempotent(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"config.yaml":            "app:\n  name: x\noracle:\n  dsn: \"oracle://u:p@db/S\"\nhooks:\n  common:\n    before: [./sql/reset.sql]\n",
		"sql/reset.sql":          "BEGIN NULL; END;",
		"runbooks/a.yml":         "desc: A-1 first\nrunners:\n  req: http://localhost:1\nsteps:\n  s:\n    test: true\n",
		"runbooks/b.yml":         "desc: A-1 duplicate id\nsteps:\n  s:\n    test: true\n",
		"runbooks/c-already.yml": "desc: c\nrunnora:\n  id: C\nsteps:\n  s:\n    test: true\n",
		"runbooks/d-flow.yml":    "runners: { req: { endpoint: 'http://localhost:1' }, db: \"oracle://u:p@db/S\" }\nsteps:\n  s:\n    test: true\n",
	})
	plan, err := Build(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(todosContaining(plan, "重複する")) != 1 {
		t.Errorf("duplicate id should be reported: %+v", plan.Todos)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); !os.IsNotExist(err) {
		t.Error("config.yaml should be removed")
	}
	p, err := project.Load(filepath.Join(dir, "runnora.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range []string{"a.yml", "b.yml", "c-already.yml", "d-flow.yml"} {
		rb, err := scenario.Read(filepath.Join(dir, "runbooks", f), p.Root)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		ids = append(ids, rb.ID)
	}
	if want := []string{"A-1", "A-1-2", "C", "d-flow"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	d, _ := os.ReadFile(filepath.Join(dir, "runbooks", "d-flow.yml"))
	if !strings.Contains(string(d), `runners: { req: { endpoint: "${API_URL}" }, db: "${ORACLE_DSN}" }`) {
		t.Errorf("flow style not replaced:\n%s", d)
	}

	again, err := Build(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Changes) != 0 {
		t.Errorf("second run should change nothing: %+v", again.Changes)
	}
}

func TestBuild_Errors(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		opts    Options
		wantErr string
	}{
		{name: "bad expectExit", files: map[string]string{"config.yaml": "app: {}\n", "s.yaml": "scenarios:\n  - runbook: a.yml\n    expectExit: 3\n", "a.yml": "steps: {}\n"}, opts: Options{ScenariosFile: "s.yaml"}, wantErr: "expectExit 3"},
		{name: "unknown key in scenarios", files: map[string]string{"config.yaml": "app: {}\n", "s.yaml": "scenarios:\n  - runbook: a.yml\n    befor: []\n"}, opts: Options{ScenariosFile: "s.yaml"}, wantErr: "befor"},
		{name: "legacy runnora.yaml", files: map[string]string{"runnora.yaml": "app: {}\n"}, wantErr: "新形式ではない"},
		{name: "duplicate env names", files: map[string]string{"config.yaml": "app: {}\n", "config.b.yaml": "app: {}\n"}, opts: Options{Envs: []EnvSpec{{Name: "x", File: "config.yaml"}, {Name: "x", File: "config.b.yaml"}}}, wantErr: "重複"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Dir = writeTree(t, tt.files)
			if _, err := Build(tt.opts); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
