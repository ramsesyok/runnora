package cmd_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestCoverageCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	covCmd, _, err := root.Find([]string{"coverage"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if covCmd == nil {
		t.Fatal("coverage command not found")
	}
}

func TestCoverageCmd_HasLongFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	covCmd, _, _ := root.Find([]string{"coverage"})
	flag := covCmd.Flags().Lookup("long")
	if flag == nil {
		t.Fatal("--long flag not found on coverage command")
	}
}

func TestCoverageCmd_HasFormatFlag(t *testing.T) {
	root := cmd.NewRootCmd()
	covCmd, _, _ := root.Find([]string{"coverage"})
	flag := covCmd.Flags().Lookup("format")
	if flag == nil {
		t.Fatal("--format flag not found on coverage command")
	}
}

func TestCoverageCmd_NoArgs_ReturnsError(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"coverage"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no path specified, got nil")
	}
}

const coverageSpec = `openapi: 3.0.3
info: {title: sample, version: 1.0.0}
paths:
  /hello:
    get:
      responses:
        "200": {description: ok}
  /other:
    get:
      responses:
        "200": {description: ok}
`

const coverageRunbook = `desc: hello
runnora:
  id: HELLO
runners:
  req:
    endpoint: ${API_URL}
    openapi3: ../openapi.yaml
steps:
  hello:
    req:
      /hello:
        get:
          body: null
`

// runners の接続先を runnora.yaml の変数で書いた runbook も、変数を展開して集計する。
func TestCoverage_ExpandsProjectVars(t *testing.T) {
	isolateEnv(t, "API_URL")
	f := newProjectFixture(t, `version: 2
defaults:
  env: unit
environments:
  unit:
    vars:
      API_URL: http://127.0.0.1:1
suites:
  all:
    select:
      paths: [runbooks/*.yml]
`)
	f.write("openapi.yaml", coverageSpec)
	book := f.write("runbooks/hello.yml", coverageRunbook)
	for _, args := range [][]string{
		{"coverage", "--project", filepath.Join(f.root, "runnora.yaml"), "--suite", "all"},
		{"coverage", "--project", filepath.Join(f.root, "runnora.yaml"), book},
		{"coverage", "--project", filepath.Join(f.root, "runnora.yaml"), filepath.Join(f.root, "runbooks", "*.yml")},
	} {
		stdout, stderr, code := f.execute(args...)
		if code != 0 || !strings.Contains(stdout, "covered: 1 / 2") {
			t.Errorf("%v: exit %d\nstdout=%s\nstderr=%s", args[3:], code, stdout, stderr)
		}
	}
	if _, _, code := f.execute("coverage", "--project", filepath.Join(f.root, "runnora.yaml"), "--suite", "all", book); code != 2 {
		t.Errorf("--suite with runbooks: exit %d, want 2", code)
	}
}
