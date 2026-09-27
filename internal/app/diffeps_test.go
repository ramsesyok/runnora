package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/scenario"
)

func diffEpsRunbook(t *testing.T) (*fixture, *scenario.Runbook) {
	t.Helper()
	f := newFixture(t)
	f.write("cases/expected.json", `{"a": 1.0, "b": {"c": 10}}`)
	f.write("cases/rules.yaml", "default:\n  abs: 0.01\n")
	f.write("runbooks/part.yml", `desc: part
vars:
  exp: json://../cases/expected.json
steps:
  inner:
    test: 'diffEps(vars.exp, {"a": 1.0, "b": {"c": 10}})'
`)
	p := f.write("runbooks/main.yml", `desc: diffEps
runnora:
  id: DIFF-1
vars:
  exp: json://../cases/expected.json
steps:
  within_inline:
    test: 'diffEps(vars.exp, {"a": 1.004, "b": {"c": 10}}, {"default": {"abs": 0.01}})'
  within_file:
    test: 'diffEps(vars.exp, {"a": 1.004, "b": {"c": 10}}, "cases/rules.yaml")'
  expect_diff:
    test: '!diffEps(vars.exp, {"a": 2, "b": {"c": 10}})'
  inc:
    include:
      path: part.yml
  broken:
    test: 'diffEps(vars.exp, {"a": 1.5, "b": {"c": 11}}, "cases/rules.yaml")'
`)
	rb, err := scenario.Read(p, f.root)
	if err != nil {
		t.Fatal(err)
	}
	return f, rb
}

func TestRun_DiffEps(t *testing.T) {
	f, rb := diffEpsRunbook(t)
	evDir := filepath.Join(t.TempDir(), "evidence")
	report, err := app.NewRunner(testSetenv(t)).Run(context.Background(), &app.Plan{
		Root: f.root, Runbooks: []*scenario.Runbook{rb}, EvidenceDir: evDir,
	})
	if exitCode(err) != 1 {
		t.Fatalf("exit %d (%v), want 1", exitCode(err), err)
	}
	steps := report.Results[0].Steps
	results := map[string]string{}
	for _, s := range steps {
		results[s.Key] = s.Result
		if s.Key != "broken" && len(s.Diffs) > 0 {
			t.Errorf("%s: only failed steps get diffs: %+v", s.Key, s.Diffs)
		}
	}
	for key, want := range map[string]string{
		"within_inline": "success", "within_file": "success", "expect_diff": "success",
		"inc": "success", "inc.inner": "success", "broken": "failure",
	} {
		if results[key] != want {
			t.Errorf("%s = %q, want %q (all: %v)", key, results[key], want, results)
		}
	}

	broken := steps[len(steps)-1]
	if broken.Key != "broken" || len(broken.Diffs) != 1 {
		t.Fatalf("broken: %+v", broken)
	}
	d := broken.Diffs[0]
	if d.Differences != 2 || len(d.Items) != 2 || d.File != "DIFF-1/05-broken.diff.json" {
		t.Fatalf("diff summary: %+v", d)
	}
	item, _ := json.Marshal(d.Items[0])
	if !strings.Contains(string(item), `"path":".a"`) || !strings.Contains(string(item), `"tolerance":{"abs":0.01`) {
		t.Errorf("first item: %s", item)
	}

	// 全件は証跡の .diff.json (runnora-diff の --format json と同じ形)。差分を期待したステップの差分も書く
	var full struct {
		Equal   bool `json:"equal"`
		Summary struct {
			Differences int `json:"differences"`
		} `json:"summary"`
	}
	b, err := os.ReadFile(filepath.Join(evDir, "DIFF-1", "05-broken.diff.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &full); err != nil || full.Equal || full.Summary.Differences != 2 {
		t.Errorf("diff file: %s", b)
	}
	if _, err := os.Stat(filepath.Join(evDir, "DIFF-1", "03-expect_diff.diff.json")); err != nil {
		t.Errorf("expected difference should still be saved: %v", err)
	}
}

// 証跡を保存しない場合も、失敗したステップの差分の要約は report.json に載る (ファイルはない)。
func TestRun_DiffEpsWithoutEvidence(t *testing.T) {
	f, rb := diffEpsRunbook(t)
	report, _ := app.NewRunner(testSetenv(t)).Run(context.Background(), &app.Plan{Root: f.root, Runbooks: []*scenario.Runbook{rb}})
	steps := report.Results[0].Steps
	broken := steps[len(steps)-1]
	if len(broken.Diffs) != 1 || broken.Diffs[0].Differences != 2 || broken.Diffs[0].File != "" {
		t.Fatalf("broken: %+v", broken)
	}
}
