package migrate

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/internal/project"
	"github.com/ramsesyok/runnora/internal/scenario"
)

// update はゴールデンファイルを作り直す (go test ./internal/migrate -run Golden -update)。
// 作り直した結果は、必ず差分をレビューしてからコミットする。
var update = flag.Bool("update", false, "update golden files")

// ゴールデンテストの入力は runnora-e2e のタグ format-v1 (旧形式) から、移行に関係するファイル
// (config*.yaml、runbooks/、scripts/) を testdata/e2e-v1/ に写したもの。
// migrate-scenarios.yaml は scripts/scenarios.psd1 と test-*.ps1 を人が書き写したシナリオ対応表。
var goldenCases = []struct {
	dir  string
	opts Options
}{
	{dir: "api-test", opts: Options{
		Envs:          []EnvSpec{{Name: "unit", File: "config.yaml"}, {Name: "mock", File: "config.mock.yaml"}},
		ScenariosFile: "migrate-scenarios.yaml",
	}},
	{dir: "grpc-test", opts: Options{
		Envs:          []EnvSpec{{Name: "unit", File: "config.yaml"}},
		ScenariosFile: "migrate-scenarios.yaml",
	}},
}

func TestGolden_E2E(t *testing.T) {
	for _, gc := range goldenCases {
		t.Run(gc.dir, func(t *testing.T) {
			work := filepath.Join(t.TempDir(), gc.dir)
			copyTree(t, filepath.Join("testdata", "e2e-v1", gc.dir), work)

			opts := gc.opts
			opts.Dir = work
			plan, err := Build(opts)
			if err != nil {
				t.Fatal(err)
			}
			if err := Apply(plan); err != nil {
				t.Fatal(err)
			}
			var report bytes.Buffer
			WriteReport(&report, plan, false)
			gotReport := strings.ReplaceAll(report.String(), filepath.ToSlash(plan.Dir), "<dir>")

			golden := filepath.Join("testdata", "e2e-v1-golden", gc.dir)
			if *update {
				if err := os.RemoveAll(golden); err != nil {
					t.Fatal(err)
				}
				for _, c := range plan.Changes {
					if c.Action == "delete" {
						continue
					}
					writeFile(t, filepath.Join(golden, filepath.FromSlash(c.Path)), c.Content)
				}
				writeFile(t, filepath.Join(golden, "MIGRATION_REPORT.txt"), gotReport)
			}

			// 変更・作成したファイルとレポートがゴールデンと一致すること
			wantFiles := map[string]bool{}
			err = filepath.WalkDir(golden, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(golden, p)
				rel = filepath.ToSlash(rel)
				want, _ := os.ReadFile(p)
				if rel == "MIGRATION_REPORT.txt" {
					if gotReport != string(want) {
						t.Errorf("report differs from golden:\n%s", gotReport)
					}
					return nil
				}
				wantFiles[rel] = true
				got, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(rel)))
				if err != nil {
					t.Errorf("%s: %v", rel, err)
				} else if string(got) != string(want) {
					t.Errorf("%s differs from golden:\n%s", rel, got)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range plan.Changes {
				switch {
				case c.Action == "delete":
					if _, err := os.Stat(filepath.Join(work, filepath.FromSlash(c.Path))); !os.IsNotExist(err) {
						t.Errorf("%s should be deleted", c.Path)
					}
				case !wantFiles[c.Path]:
					t.Errorf("%s changed but has no golden file (run with -update and review)", c.Path)
				}
			}

			// 移行結果は新形式として読めること (runnora.yaml、全スイートの選択、runnora: ブロック)
			p, err := project.Load(filepath.Join(work, project.FileName))
			if err != nil {
				t.Fatalf("migrated runnora.yaml: %v", err)
			}
			for _, name := range p.SuiteNames() {
				rbs, err := scenario.Select(p.Root, p.Suites[name].Select)
				if err != nil || len(rbs) == 0 {
					t.Errorf("suite %s: %d runbooks, %v", name, len(rbs), err)
				}
				env, err := p.SelectEnv("", name)
				if err != nil {
					t.Fatalf("suite %s: %v", name, err)
				}
				if _, err := p.Resolve(env, name, nil, func(string) (string, bool) { return "", false }); err != nil {
					t.Errorf("suite %s: %v", name, err)
				}
			}

			// もう一度実行しても変更はない
			again, err := Build(Options{Dir: work})
			if err != nil {
				t.Fatal(err)
			}
			if len(again.Changes) != 0 {
				t.Errorf("second run should change nothing: %+v", again.Changes)
			}
		})
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		writeFile(t, filepath.Join(dst, rel), string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
