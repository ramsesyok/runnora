package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const evidenceProject = `version: 2
defaults:
  env: unit
environments:
  unit:
    vars:
      API_URL: %URL%
suites:
  all:
    select:
      paths: [runbooks/*.yml]
evidence:
  mask:
    headers: [X-Secret]
`

// dumpRunbook は旧来の dump ステップ ({{ env.RUNNORA_EVIDENCE_DIR }}) を持つ runbook。
const dumpRunbook = `desc: old style
runnora:
  id: OLD-1
runners:
  req: ${API_URL}
steps:
  hello:
    req:
      /hello:
        get:
          headers:
            X-Secret: s3cret
          body: null
  dump_hello:
    dump:
      expr: steps.hello.res.status
      out: '{{ env.RUNNORA_EVIDENCE_DIR }}/hello-status.txt'
`

// reportDirs は reports/ の下にできた実行ごとのフォルダを返す。
func reportDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "reports"))
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(root, "reports", e.Name()))
		}
	}
	return dirs
}

func TestRun_EvidenceAndRunFolder(t *testing.T) {
	isolateEnv(t, "API_URL", "RUNNORA_EVIDENCE_DIR")
	srv := okServer(t)
	f := newProjectFixture(t, strings.ReplaceAll(evidenceProject, "%URL%", srv.URL))
	f.write("runbooks/a.yml", scenarioRunbook("SC-1", "", "/hello"))
	f.write("runbooks/old.yml", dumpRunbook)
	projectFile := filepath.Join(f.root, "runnora.yaml")

	_, stderr, code := f.execute("run", "--project", projectFile, "--suite", "all")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	dirs := reportDirs(t, f.root)
	if len(dirs) != 1 || !strings.HasSuffix(dirs[0], "-all") {
		t.Fatalf("run folders: %v", dirs)
	}
	run := dirs[0]
	if !strings.Contains(stderr, "レポート:") || !strings.Contains(stderr, "report.json") {
		t.Errorf("stderr should show the report path: %s", stderr)
	}
	if !strings.Contains(stderr, "runbooks/old.yml: dump ステップは証跡の自動保存と重複しています") {
		t.Errorf("stderr should warn about dump steps: %s", stderr)
	}

	// report.json: 証跡のフォルダとファイルの一覧
	var rep struct {
		Suite       string `json:"suite"`
		EvidenceDir string `json:"evidenceDir"`
		Results     []struct {
			ID       string `json:"id"`
			Evidence []struct {
				Key  string `json:"key"`
				Path string `json:"path"`
			} `json:"evidence"`
		} `json:"results"`
	}
	b, err := os.ReadFile(filepath.Join(run, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Suite != "all" || rep.EvidenceDir != "evidence" || len(rep.Results) != 2 {
		t.Fatalf("report.json: %s", b)
	}
	byID := map[string]string{}
	for _, r := range rep.Results {
		if len(r.Evidence) != 1 || r.Evidence[0].Key != "hello" {
			t.Fatalf("%s evidence: %+v", r.ID, r.Evidence)
		}
		byID[r.ID] = r.Evidence[0].Path
	}
	if byID["SC-1"] != "SC-1/01-hello.json" || byID["OLD-1"] != "OLD-1/01-hello.json" {
		t.Errorf("evidence paths: %v", byID)
	}

	// 自動保存した証跡 (応答だけ。既定のモード)
	var ev map[string]any
	b, err = os.ReadFile(filepath.Join(run, "evidence", "SC-1", "01-hello.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	if ev["scenarioId"] != "SC-1" || ev["request"] != nil || ev["response"].(map[string]any)["status"].(float64) != 200 {
		t.Errorf("evidence: %s", b)
	}

	// 旧来の dump ステップは、そのシナリオの証跡フォルダに書く (RUNNORA_EVIDENCE_DIR)
	if got, err := os.ReadFile(filepath.Join(run, "evidence", "OLD-1", "hello-status.txt")); err != nil || !strings.Contains(string(got), "200") {
		t.Errorf("dump output: %q, %v", got, err)
	}

	// 2 回目は別のフォルダ (同じ秒なら -2 が付く)
	if _, stderr, code := f.execute("run", "--project", projectFile, "--suite", "all"); code != 0 {
		t.Fatalf("second run: exit %d: %s", code, stderr)
	}
	if dirs := reportDirs(t, f.root); len(dirs) != 2 {
		t.Fatalf("second run should create another folder: %v", dirs)
	}
}

func TestRun_EvidenceFlags(t *testing.T) {
	isolateEnv(t, "API_URL", "RUNNORA_EVIDENCE_DIR")
	srv := okServer(t)
	f := newProjectFixture(t, strings.ReplaceAll(evidenceProject, "%URL%", srv.URL)+"report:\n  dir: out/runs\n")
	book := f.write("runbooks/a.yml", scenarioRunbook("SC-1", "", "/hello"))
	projectFile := filepath.Join(f.root, "runnora.yaml")

	t.Run("no-evidence", func(t *testing.T) {
		if _, stderr, code := f.execute("run", "--project", projectFile, "--no-evidence", book); code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		entries, _ := os.ReadDir(filepath.Join(f.root, "out", "runs"))
		if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), "-run") {
			t.Fatalf("report.dir: %v", entries)
		}
		run := filepath.Join(f.root, "out", "runs", entries[0].Name())
		if _, err := os.Stat(filepath.Join(run, "report.json")); err != nil {
			t.Error(err)
		}
		if _, err := os.Stat(filepath.Join(run, "evidence")); !os.IsNotExist(err) {
			t.Error("--no-evidence should not create evidence/")
		}
	})

	t.Run("evidence-dir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "ev")
		if _, stderr, code := f.execute("run", "--project", projectFile, "--evidence-dir", dir, book); code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, "SC-1", "01-hello.json")); err != nil {
			t.Error(err)
		}
	})

	t.Run("failure before running removes the empty folder", func(t *testing.T) {
		before, _ := os.ReadDir(filepath.Join(f.root, "out", "runs"))
		f.write("runbooks/c.yml", "desc: c\nrunnora:\n  id: C\n  before: [missing.sql]\nsteps:\n  s:\n    test: true\n")
		_, _, code := f.execute("run", "--project", projectFile, filepath.Join(f.root, "runbooks", "c.yml"))
		if code == 0 {
			t.Fatal("missing SQL should fail")
		}
		after, _ := os.ReadDir(filepath.Join(f.root, "out", "runs"))
		if len(after) != len(before) {
			t.Errorf("empty run folder left behind: %d -> %d", len(before), len(after))
		}
	})
}

func TestValidate_WarnsDumpSteps(t *testing.T) {
	isolateEnv(t, "API_URL")
	f := newProjectFixture(t, strings.ReplaceAll(evidenceProject, "%URL%", "http://127.0.0.1:1"))
	f.write("runbooks/old.yml", dumpRunbook)
	f.write("runbooks/a.yml", scenarioRunbook("SC-1", "", "/hello"))
	stdout, stderr, code := f.execute("validate", "--project", filepath.Join(f.root, "runnora.yaml"))
	if code != 0 {
		t.Fatalf("warnings must not fail validate: exit %d\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "dump ステップは証跡の自動保存と重複しています") || strings.Count(stdout, "dump ステップ") != 1 {
		t.Errorf("validate output:\n%s", stdout)
	}
}
