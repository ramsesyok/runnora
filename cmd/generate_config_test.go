package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestGenerateRejectsMalformedConfigBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "runnora.yaml")
	openAPIPath := filepath.Join(dir, "openapi.yaml")
	outDir := filepath.Join(dir, "output")
	if err := os.WriteFile(configPath, []byte("version: 2\ngenerate: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openAPIPath, []byte("openapi: 3.0.3\ninfo: {title: test, version: '1'}\npaths: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"generate", "--project", configPath, "--openapi", openAPIPath, "--out", outDir})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "project:") {
		t.Fatalf("malformed config should fail, got %v", err)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("output created despite malformed config: %v", err)
	}
}
