package cmd_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

// writeRunbook はテスト用 runbook ファイルを作成して返す。
func writeRunbookFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write runbook %s: %v", name, err)
	}
	return p
}

func simpleRunbook(serverURL string) string {
	return fmt.Sprintf(`
desc: hello test
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
}

func TestListCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	listCmd, _, err := root.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if listCmd == nil {
		t.Fatal("list command not found")
	}
}

func TestListCmd_HasAlias_ls(t *testing.T) {
	root := cmd.NewRootCmd()
	listCmd, _, _ := root.Find([]string{"list"})
	found := false
	for _, a := range listCmd.Aliases {
		if a == "ls" {
			found = true
		}
	}
	if !found {
		t.Error("list command should have 'ls' alias")
	}
}

func TestListCmd_OutputsRunbookPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	p := writeRunbookFile(t, dir, "hello.yml", simpleRunbook(srv.URL))

	root := cmd.NewRootCmd()
	root.SetArgs([]string{"list", p})
	var out bytes.Buffer
	root.SetOut(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "hello.yml") {
		t.Errorf("output should contain runbook filename: %q", out.String())
	}
}

func TestListCmd_JSONFormat_ContainsPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	p := writeRunbookFile(t, dir, "hello.yml", simpleRunbook(srv.URL))

	root := cmd.NewRootCmd()
	root.SetArgs([]string{"list", "--format", "json", p})
	var out bytes.Buffer
	root.SetOut(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "path") {
		t.Errorf("JSON output should contain 'path' key: %q", out.String())
	}
}

func TestListCmd_LongFlag_ShowsFullID(t *testing.T) {
	root := cmd.NewRootCmd()
	listCmd, _, _ := root.Find([]string{"list"})
	flag := listCmd.Flags().Lookup("long")
	if flag == nil {
		t.Fatal("--long flag not found on list command")
	}
}
