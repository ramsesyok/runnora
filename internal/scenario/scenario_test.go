package scenario

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/project"
)

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func scenarioYAML(id string, labels ...string) string {
	return "desc: " + id + "\nrunnora:\n  id: " + id + "\nlabels: [" + strings.Join(labels, ", ") + "]\nsteps:\n  s:\n    test: true\n"
}

func TestRead(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name       string
		content    string
		wantID     string
		wantExpect string
		wantMeta   bool
		wantErr    string
	}{
		{name: "block", content: "runnora:\n  id: LIB-004\n  before: [sql/a.sql]\n  expect: hookFail\nsteps: {}\n", wantID: "LIB-004", wantExpect: OutcomeHookFail, wantMeta: true},
		{name: "no block derives id from path", content: "desc: x\nsteps: {}\n", wantID: "runbooks/demo/no_block", wantExpect: OutcomePass},
		{name: "expect defaults to pass", content: "runnora:\n  id: A\nsteps: {}\n", wantID: "A", wantExpect: OutcomePass, wantMeta: true},
		{name: "missing id", content: "runnora:\n  expect: pass\n", wantErr: "id がありません"},
		{name: "bad id", content: "runnora:\n  id: \"a b\"\n", wantErr: "使えない文字"},
		{name: "bad expect", content: "runnora:\n  id: A\n  expect: ok\n", wantErr: "expect \"ok\""},
		{name: "env placeholders stay valid yaml", content: "runnora:\n  id: A\nrunners:\n  req:\n    endpoint: ${API_URL}\n  db: ${ORACLE_DSN}\n", wantID: "A", wantExpect: OutcomePass, wantMeta: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := "runbooks/demo/no_block.yml"
			if i != 1 {
				rel = "runbooks/case" + string(rune('a'+i)) + ".yml"
			}
			p := writeFile(t, root, rel, tt.content)
			rb, err := Read(p, root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rb.ID != tt.wantID || rb.Expect() != tt.wantExpect || (rb.Meta != nil) != tt.wantMeta {
				t.Errorf("got id=%q expect=%q meta=%v", rb.ID, rb.Expect(), rb.Meta != nil)
			}
		})
	}
}

func TestSQLFilesAndAllowedIn(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, "runbooks/x.yml", "runnora:\n  id: X\n  before: [sql/b.sql]\n  after: [sql/a.sql]\n  envs: [unit]\n")
	rb, err := Read(p, root)
	if err != nil {
		t.Fatal(err)
	}
	before, after := rb.SQLFiles(root)
	if !reflect.DeepEqual(before, []string{filepath.Join(root, "sql", "b.sql")}) || !reflect.DeepEqual(after, []string{filepath.Join(root, "sql", "a.sql")}) {
		t.Errorf("sql files: %v %v", before, after)
	}
	if !rb.AllowedIn("unit") || rb.AllowedIn("mock") {
		t.Error("envs filter wrong")
	}
	noBlock := &Runbook{}
	if !noBlock.AllowedIn("mock") {
		t.Error("runbook without block should run everywhere")
	}
}

func TestSelect(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "runbooks/scenarios/lib-001.yml", scenarioYAML("LIB-001", "scenario"))
	writeFile(t, root, "runbooks/scenarios/lib-002.yml", scenarioYAML("LIB-002", "scenario", "slow"))
	writeFile(t, root, "runbooks/scenarios/lib-003.yml", scenarioYAML("LIB-003", "other"))
	writeFile(t, root, "runbooks/scenarios/part.template.yml", "desc: part\nsteps:\n  s:\n    test: true\n")
	writeFile(t, root, "runbooks/scenarios/nested/lib-010.yml", scenarioYAML("LIB-010", "scenario"))
	writeFile(t, root, ".hidden/lib-999.yml", scenarioYAML("LIB-999", "scenario"))

	ids := func(rbs []*Runbook) []string {
		var out []string
		for _, rb := range rbs {
			out = append(out, rb.ID)
		}
		return out
	}
	tests := []struct {
		name    string
		sel     project.Selection
		want    []string
		wantErr string
	}{
		{name: "single star skips template and nested", sel: project.Selection{Paths: []string{"runbooks/scenarios/*.yml"}}, want: []string{"LIB-001", "LIB-002", "LIB-003"}},
		{name: "double star includes nested", sel: project.Selection{Paths: []string{"runbooks/**/*.yml"}}, want: []string{"LIB-001", "LIB-002", "LIB-003", "LIB-010"}},
		{name: "leading ./ is ignored", sel: project.Selection{Paths: []string{"./runbooks/scenarios/lib-00?.yml"}}, want: []string{"LIB-001", "LIB-002", "LIB-003"}},
		{name: "labels", sel: project.Selection{Paths: []string{"runbooks/**/*.yml"}, Labels: []string{"scenario"}}, want: []string{"LIB-001", "LIB-002", "LIB-010"}},
		{name: "ids keep given order", sel: project.Selection{Paths: []string{"runbooks/**/*.yml"}, IDs: []string{"LIB-010", "LIB-001"}}, want: []string{"LIB-010", "LIB-001"}},
		{name: "missing id", sel: project.Selection{Paths: []string{"runbooks/scenarios/*.yml"}, IDs: []string{"LIB-001", "LIB-404"}}, wantErr: "LIB-404"},
		{name: "hidden dirs are skipped", sel: project.Selection{Paths: []string{"**/*.yml"}, IDs: []string{"LIB-999"}}, wantErr: "LIB-999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Select(root, tt.sel)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ids(got), tt.want) {
				t.Errorf("got %v, want %v", ids(got), tt.want)
			}
		})
	}

	writeFile(t, root, "runbooks/dup/a.yml", scenarioYAML("DUP"))
	writeFile(t, root, "runbooks/dup/b.yml", scenarioYAML("DUP"))
	if _, err := Select(root, project.Selection{Paths: []string{"runbooks/dup/*.yml"}}); err == nil || !strings.Contains(err.Error(), "重複") {
		t.Errorf("duplicate id should fail: %v", err)
	}
}

func TestFromArgs(t *testing.T) {
	root := t.TempDir()
	a := writeFile(t, root, "runbooks/a.yml", scenarioYAML("A"))
	writeFile(t, root, "runbooks/b.yml", "desc: b\nsteps: {}\n")
	writeFile(t, root, "runbooks/sub/c.yml", scenarioYAML("C"))

	rbs, err := FromArgs([]string{filepath.Join(root, "runbooks", "*.yml"), a}, root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, rb := range rbs {
		got = append(got, rb.ID)
	}
	if want := []string{"A", "runbooks/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (duplicates removed, block-less runbook kept)", got, want)
	}

	rbs, err = FromArgs([]string{filepath.Join(root, "runbooks", "**", "*.yml")}, root)
	if err != nil || len(rbs) != 3 {
		t.Errorf("double star: %d runbooks, %v", len(rbs), err)
	}
	if _, err := FromArgs([]string{filepath.Join(root, "missing.yml")}, root); err == nil {
		t.Error("missing runbook should fail")
	}
	if _, err := FromArgs([]string{filepath.Join(root, "none", "*.yml")}, root); err == nil {
		t.Error("glob without matches should fail")
	}
}

func TestIncludes(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, "runbooks/suite.yml", `desc: suite
steps:
  a:
    include: ./a.template.yml
  b:
    include:
      path: ../shared/b.yml
      vars:
        case: json://x.json
  c:
    include:
      path: "{{ vars.dynamic }}"
  d:
    req:
      /x:
        get: {}
`)
	rb, err := Read(p, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "runbooks", "a.template.yml"), filepath.Join(root, "shared", "b.yml")}
	if got := Includes(rb); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
