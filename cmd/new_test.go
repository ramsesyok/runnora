package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramsesyok/runnora/cmd"
)

func TestNewCmd_Exists(t *testing.T) {
	root := cmd.NewRootCmd()
	newCmd, _, err := root.Find([]string{"new"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if newCmd == nil {
		t.Fatal("new command not found")
	}
}

func TestNewCmd_HasAlias_append(t *testing.T) {
	root := cmd.NewRootCmd()
	newCmd, _, _ := root.Find([]string{"new"})
	found := false
	for _, a := range newCmd.Aliases {
		if a == "append" {
			found = true
		}
	}
	if !found {
		t.Error("new command should have 'append' alias")
	}
}

func TestNewCmd_WithStep_OutputsYAML(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"new", "GET", "https://example.com/hello"})
	var out bytes.Buffer
	root.SetOut(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "steps") {
		t.Errorf("output should contain 'steps': %q", output)
	}
}

func TestNewCmd_WithDescFlag_OutputsDesc(t *testing.T) {
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"new", "--desc", "My Test Runbook", "GET", "https://example.com/hello"})
	var out bytes.Buffer
	root.SetOut(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "My Test Runbook") {
		t.Errorf("output should contain desc: %q", output)
	}
}

func TestNewCmd_WithOutFlag_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.yml")

	root := cmd.NewRootCmd()
	root.SetArgs([]string{"new", "--out", outFile, "GET", "https://example.com/hello"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), "steps") {
		t.Errorf("file should contain 'steps': %q", string(content))
	}
}
