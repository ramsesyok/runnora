package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
	"github.com/spf13/pflag"
)

func TestGenmockCmd_RegisteredWithSubcommands(t *testing.T) {
	root := cmd.NewRootCmd()
	genmockCmd, _, err := root.Find([]string{"genmock"})
	if err != nil {
		t.Fatalf("find genmock command: %v", err)
	}
	if genmockCmd == nil {
		t.Fatal("genmock command not found")
	}

	for _, name := range []string{"init", "build", "validate"} {
		sub, _, err := root.Find([]string{"genmock", name})
		if err != nil {
			t.Fatalf("find genmock %s command: %v", name, err)
		}
		if sub == nil {
			t.Fatalf("genmock %s command not found", name)
		}
	}
}

func TestGenmockCmd_DefaultFlags(t *testing.T) {
	root := cmd.NewRootCmd()

	initCmd, _, _ := root.Find([]string{"genmock", "init"})
	if flag := initCmd.Flags().Lookup("out-cases"); flag == nil || flag.DefValue != "mock-cases.yaml" {
		t.Fatalf("genmock init --out-cases default = %v, want mock-cases.yaml", flagDefault(flag))
	}
	if flag := initCmd.Flags().Lookup("responses-root"); flag == nil || flag.DefValue != "mock-responses" {
		t.Fatalf("genmock init --responses-root default = %v, want mock-responses", flagDefault(flag))
	}

	buildCmd, _, _ := root.Find([]string{"genmock", "build"})
	if flag := buildCmd.Flags().Lookup("cases"); flag == nil || flag.DefValue != "mock-cases.yaml" {
		t.Fatalf("genmock build --cases default = %v, want mock-cases.yaml", flagDefault(flag))
	}
	if flag := buildCmd.Flags().Lookup("responses-root"); flag == nil || flag.DefValue != "mock-responses" {
		t.Fatalf("genmock build --responses-root default = %v, want mock-responses", flagDefault(flag))
	}
	if flag := buildCmd.Flags().Lookup("out"); flag == nil || flag.DefValue != "wiremock-out" {
		t.Fatalf("genmock build --out default = %v, want wiremock-out", flagDefault(flag))
	}
	for _, name := range []string{"clean", "strict", "fail-on-missing-operation", "fail-on-missing-body-file", "no-auto-fallback"} {
		if flag := buildCmd.Flags().Lookup(name); flag == nil {
			t.Fatalf("genmock build --%s flag not found", name)
		}
	}

	validateCmd, _, _ := root.Find([]string{"genmock", "validate"})
	for _, name := range []string{"strict", "fail-on-missing-operation", "fail-on-missing-body-file"} {
		if flag := validateCmd.Flags().Lookup(name); flag == nil {
			t.Fatalf("genmock validate --%s flag not found", name)
		}
	}
}

func TestGenmockCmd_OpenAPIRequired(t *testing.T) {
	for _, args := range [][]string{
		{"genmock", "init"},
		{"genmock", "build"},
		{"genmock", "validate"},
	} {
		root := cmd.NewRootCmd()
		root.SetArgs(args)
		var errBuf bytes.Buffer
		root.SetErr(&errBuf)

		err := root.Execute()
		if err == nil {
			t.Fatalf("%v: expected required --openapi error, got nil", args)
		}
		if !strings.Contains(err.Error(), "required flag(s) \"openapi\" not set") {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}
	}
}

func TestGenmockValidate_IntegrationOK(t *testing.T) {
	dir := t.TempDir()
	openAPIPath, casesPath, responsesRoot := writeGenmockFixture(t, dir)

	root := cmd.NewRootCmd()
	root.SetArgs([]string{
		"genmock", "validate",
		"--openapi", openAPIPath,
		"--cases", casesPath,
		"--responses-root", responsesRoot,
		"--fail-on-missing-operation",
		"--fail-on-missing-body-file",
	})
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "OK: no issues found") {
		t.Fatalf("validate output = %q, want OK", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %s", errOut.String())
	}
}

func TestGenmockBuild_IntegrationWritesWireMockFiles(t *testing.T) {
	dir := t.TempDir()
	openAPIPath, casesPath, responsesRoot := writeGenmockFixture(t, dir)
	outDir := filepath.Join(dir, "wiremock-out")

	root := cmd.NewRootCmd()
	root.SetArgs([]string{
		"genmock", "build",
		"--openapi", openAPIPath,
		"--cases", casesPath,
		"--responses-root", responsesRoot,
		"--out", outDir,
		"--no-auto-fallback",
		"--fail-on-missing-operation",
		"--fail-on-missing-body-file",
	})
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "build complete -> "+outDir) {
		t.Fatalf("build output = %q, want completion line", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, "mappings", "getWidget__getWidget_default.json")); err != nil {
		t.Fatalf("mapping was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "__files", "getWidget", "getWidget_default.json")); err != nil {
		t.Fatalf("body file was not copied: %v", err)
	}
}

func TestGenmockInit_IntegrationWritesCasesAndStubs(t *testing.T) {
	dir := t.TempDir()
	openAPIPath := filepath.Join(dir, "openapi.yaml")
	outCasesPath := filepath.Join(dir, "mock-cases.yaml")
	responsesRoot := filepath.Join(dir, "mock-responses")
	writeFile(t, openAPIPath, genmockOpenAPI)

	root := cmd.NewRootCmd()
	root.SetArgs([]string{
		"genmock", "init",
		"--openapi", openAPIPath,
		"--out-cases", outCasesPath,
		"--responses-root", responsesRoot,
	})
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "wrote case YAML -> "+outCasesPath) {
		t.Fatalf("init output = %q, want wrote case YAML line", got)
	}
	if _, err := os.Stat(outCasesPath); err != nil {
		t.Fatalf("case YAML was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(responsesRoot, "getWidget", "getWidget_default.json")); err != nil {
		t.Fatalf("response stub was not written: %v", err)
	}
}

func flagDefault(flag *pflag.Flag) string {
	if flag == nil {
		return "<nil>"
	}
	return flag.DefValue
}

func writeGenmockFixture(t *testing.T, dir string) (string, string, string) {
	t.Helper()

	openAPIPath := filepath.Join(dir, "openapi.yaml")
	casesPath := filepath.Join(dir, "mock-cases.yaml")
	responsesRoot := filepath.Join(dir, "mock-responses")

	writeFile(t, openAPIPath, genmockOpenAPI)
	writeFile(t, casesPath, genmockCases)
	writeFile(t, filepath.Join(responsesRoot, "getWidget", "getWidget_default.json"), `{"id":"w1","name":"Widget"}`)

	return openAPIPath, casesPath, responsesRoot
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const genmockOpenAPI = `openapi: 3.0.3
info:
  title: Widget API
  version: 1.0.0
paths:
  /widgets/{id}:
    get:
      operationId: getWidget
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  name:
                    type: string
`

const genmockCases = `version: 1
cases:
  - id: getWidget_default
    operationId: getWidget
    priority: 10
    request:
      pathParams:
        id:
          equalTo: "w1"
    response:
      status: 200
      bodyFile: getWidget/getWidget_default.json
`
